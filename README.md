# TLDR Bot

@tldr is an open-source bot for Mastodon designed to make long posts and threads less overwhelming. It generates concise summaries (TL;DRs) for posts and threads, helping users quickly grasp the essentials without reading walls of text.

## How It Works

TLDR Bot listens for mentions and follows on Mastodon. If a follower posts a long message without a TL;DR, the bot steps in and generates one automatically. It can also summarize entire threads when mentioned, keeping the conversation easy to follow.

### Features

- **Automatic TL;DRs for Followers:**  
  If you follow @tldr and post something long without a "tl;dr," the bot will generate one for you.
- **Thread Summarization:**  
  Mention @tldr in any thread, and it will summarize the entire conversation for you.
- **Follow-Backs:**  
  @tldr follows back anyone who follows it.
- **Visibility Matching:**  
  Replies match the original post’s visibility (but public posts are kept unlisted to stay subtle).

## Setup

1. **Clone the repository:**
   ```sh
   git clone https://github.com/micr0-dev/tldr-bot.git
   cd tldr-bot
   ```

2. **Configure the bot:**
   Copy the example configuration file and edit it:
   ```sh
   cp example.config.toml config.toml
   ```

   Open `config.toml` in your preferred text editor and set the following values:

   ```toml
   [server]
   mastodon_server = "https://mastodon.example.com"  # Your Mastodon instance URL
   client_secret = "your_client_secret"              # Mastodon app client secret
   access_token = "your_access_token"                # Mastodon access token

   [llm]
   provider = "gemini"          # or "ollama"
   ollama_model = "llava-phi3"  # Local model (if using Ollama)

   [gemini]
   api_key = "your_gemini_api_key"  # Gemini API key from https://aistudio.google.com
   model = "gemini-1.5-flash"       # Choose from available models
   ```

3. **Install dependencies:**
   ```sh
   go mod tidy
   ```

4. **Run the bot:**
   ```sh
   go run main.go
   ```

## Contributing

Contributions are welcome! If you have ideas, improvements, or bug fixes, open an issue or submit a pull request.

## License

This project is licensed under the [OVERWORKED LICENSE (OWL) v1.0](https://owl-license.org/). See the [LICENSE](LICENSE) file for more details.

---

Make your long posts manageable and your threads easy to follow—one TL;DR at a time!