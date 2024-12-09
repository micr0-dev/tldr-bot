package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/google/generative-ai-go/genai"
	"github.com/mattn/go-mastodon"
	"golang.org/x/net/html"
	"google.golang.org/api/option"
)

type Config struct {
	Server struct {
		MastodonServer string `toml:"mastodon_server"`
		ClientSecret   string `toml:"client_secret"`
		AccessToken    string `toml:"access_token"`
	} `toml:"server"`
	LLM struct {
		Provider    string `toml:"provider"`
		OllamaModel string `toml:"ollama_model"`
	} `toml:"llm"`
	Gemini struct {
		APIKey string `toml:"api_key"`
	} `toml:"gemini"`
}

var config Config
var model *genai.GenerativeModel
var ctx context.Context

func main() {
	// Load configuration
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		log.Fatalf("Error loading config.toml: %v", err)
	}

	ctx = context.Background()
	c := mastodon.NewClient(&mastodon.Config{
		Server:       config.Server.MastodonServer,
		ClientSecret: config.Server.ClientSecret,
		AccessToken:  config.Server.AccessToken,
	})

	// Set up AI model
	if err := SetupModel(config.Gemini.APIKey); err != nil {
		log.Fatalf("Error setting up AI model: %v", err)
	}

	ws := c.NewWSClient()

	// Connect to Mastodon Streaming API
	events, err := ws.StreamingWSUser(ctx)
	if err != nil {
		log.Fatalf("Error connecting to streaming API: %v", err)
	}
	fmt.Println("Thread Summarizer Bot is running...")

	// Event loop to listen for mentions and follows
	for event := range events {
		switch e := event.(type) {
		case *mastodon.NotificationEvent:
			if e.Notification.Type == "mention" {
				if e.Notification.Account.Bot {
					break
				}
				handleMention(c, e.Notification)
			} else if e.Notification.Type == "follow" {
				if e.Notification.Account.Bot {
					break
				}
				handleFollowBack(c, e.Notification.Account.ID)
			}
		case *mastodon.UpdateEvent:
			if e.Status.Account.Bot {
				break
			}
			checkForLongPost(c, e.Status)
		}
	}
}

// SetupModel initializes the Gemini AI model
func SetupModel(apiKey string) error {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return err
	}
	model = client.GenerativeModel("gemini-1.5-flash")
	return nil
}

// handleFollowBack follows back new followers
func handleFollowBack(c *mastodon.Client, userID mastodon.ID) {
	_, err := c.AccountFollow(ctx, userID)
	if err != nil {
		log.Printf("Error following back: %v", err)
	} else {
		fmt.Printf("Followed back user: %s\n", userID)
	}
}

// handleMention processes mentions and posts a summarized response
func handleMention(c *mastodon.Client, notification *mastodon.Notification) {
	if notification.Account.Acct == os.Getenv("MASTODON_USERNAME") || notification.Status.Account.Bot {
		return
	}

	thread, err := fetchThread(c, notification.Status)
	if err != nil {
		log.Printf("Error fetching thread: %v", err)
		return
	}

	summary, err := summarizeThread(thread, false)
	if err != nil {
		log.Printf("Error summarizing thread: %v", err)
		summary = "uh oh, something went wrong. can't summarize this thread.\n" + err.Error()
	}

	summary = cleanResponse(summary)

	response := fmt.Sprintf("@%s TL;DR: %s", notification.Account.Acct, summary)

	visibility := notification.Status.Visibility
	if visibility == "public" {
		visibility = "unlisted"
	}

	// Prepare the content warning for the reply
	contentWarning := notification.Status.SpoilerText
	if contentWarning != "" && !strings.HasPrefix(contentWarning, "re:") {
		contentWarning = "re: " + contentWarning
	}

	_, err = c.PostStatus(ctx, &mastodon.Toot{
		Status:      response,
		InReplyToID: notification.Status.ID,
		Visibility:  visibility,
		SpoilerText: contentWarning,
	})
	if err != nil {
		log.Printf("Error posting summary: %v", err)
	} else {
		fmt.Printf("Posted summary: %s\n", response)
	}
}

// checkForLongPost checks if a post is long and needs a TL;DR
func checkForLongPost(c *mastodon.Client, status *mastodon.Status) {
	content := extractTextFromHTML(status.Content)
	wordCount := countWords(content)

	if wordCount > 200 && !strings.Contains(strings.ToLower(content), "tl;dr") {
		summary, err := summarizeThread(content, true)
		if err != nil {
			log.Printf("Error generating TL;DR: %v", err)
			return
		}

		summary = cleanResponse(summary)

		response := fmt.Sprintf("@%s TL;DR: %s", status.Account.Acct, summary)

		// Prepare the content warning for the reply
		contentWarning := status.SpoilerText
		if contentWarning != "" && !strings.HasPrefix(contentWarning, "re:") {
			contentWarning = "re: " + contentWarning
		}

		_, err = c.PostStatus(ctx, &mastodon.Toot{
			Status:      response,
			InReplyToID: status.ID,
			Visibility:  status.Visibility,
			SpoilerText: contentWarning,
		})
		if err != nil {
			log.Printf("Error posting TL;DR: %v", err)
		} else {
			fmt.Printf("Posted TL;DR for user %s\n", status.Account.Acct)
		}
	}
}

func cleanResponse(response string) string {
	// Compile a regex to match common TL;DR patterns (case-insensitive)
	tldrRegex := regexp.MustCompile(`(?i)(^|\s)?tl;dr[:\-\s]*`)

	// Remove any redundant TL;DR prefixes or headings
	response = tldrRegex.ReplaceAllString(response, "")

	// Remove markdown-style headers (e.g., ## Heading)
	headerRegex := regexp.MustCompile(`(?m)^#+\s*.*\n?`)
	response = headerRegex.ReplaceAllString(response, "")

	// Clean up any double spaces or unnecessary punctuation spacing
	response = strings.ReplaceAll(response, ".  ", ". ")
	response = strings.ReplaceAll(response, ",  ", ", ")

	// Trim leading and trailing spaces
	response = strings.TrimSpace(response)

	return response
}

// fetchThread gathers the entire thread up to the root post
func fetchThread(c *mastodon.Client, status *mastodon.Status) (string, error) {
	var thread []string
	currentStatus := status

	for currentStatus != nil {
		content := extractTextFromHTML(currentStatus.Content)
		thread = append([]string{fmt.Sprintf("%s: %s", currentStatus.Account.Username, content)}, thread...)

		if currentStatus.InReplyToID == nil {
			break
		}

		parentID := mastodon.ID(fmt.Sprintf("%v", currentStatus.InReplyToID))
		parentStatus, err := c.GetStatus(ctx, parentID)
		if err != nil {
			log.Printf("Error fetching parent status: %v", err)
			break
		}
		currentStatus = parentStatus
	}

	return strings.Join(thread, "\n\n"), nil
}

// extractTextFromHTML extracts plain text from HTML content
func extractTextFromHTML(content string) string {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		log.Printf("Error parsing HTML: %v", err)
		return content
	}
	var extractText func(*html.Node) string
	extractText = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var result string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			result += extractText(c)
		}
		return result
	}
	return extractText(doc)
}

// summarizeThread uses the AI model to summarize the thread
func summarizeThread(thread string, isSinglePost bool) (string, error) {
	var prompt string
	if !isSinglePost {
		prompt = fmt.Sprintf("Write a TL;DR summary for this conversation. Reply with just the TL;DR and nothing else:\n%s", thread)
	} else {
		prompt = fmt.Sprintf("Write a TL;DR summary for this post. Refer to the Original poster as OP. Reply with just the TL;DR and nothing else:\n%s", thread)
	}
	switch config.LLM.Provider {
	case "gemini":
		return generateWithGemini(prompt)
	case "ollama":
		return generateWithOllama(prompt)
	default:
		return "", fmt.Errorf("unsupported LLM provider: %s", config.LLM.Provider)
	}
}

// generateWithGemini sends a prompt to the Gemini model
func generateWithGemini(prompt string) (string, error) {
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}
	return getResponse(resp), nil
}

// generateWithOllama runs the Ollama command
func generateWithOllama(prompt string) (string, error) {
	cmd := exec.Command("ollama", "run", config.LLM.OllamaModel, prompt)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// getResponse extracts the response from the AI model
func getResponse(resp *genai.GenerateContentResponse) string {
	var response string
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				response += fmt.Sprintf("%v", part)
			}
		}
	}
	return response
}

// countWords counts the words in a string
func countWords(text string) int {
	inWord := false
	count := 0

	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}
