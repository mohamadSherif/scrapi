package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/gocolly/colly"
	"github.com/joho/godotenv"
)

// ScrapedPage holds the URL and the content of the scraped page
type ScrapedPage struct {
	URL     string
	Content string
	Prompt  string
	Proxy   bool
}

type ProxyResponse struct {
	Data []struct {
		IPPort string `json:"ipPort"`
	} `json:"data"`
}

func goDotEnvVariable(key string) string {

	// load .env file
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	return os.Getenv(key)
}

func main() {
	asciiart := `
 ___________________________________________________________
|                                                           |
|     /$$$$$$                                         /$$   |
|    /$$__  $$                                       |__/   |
|   | $$  \__/  /$$$$$$$  /$$$$$$  /$$$$$$   /$$$$$$  /$$   |
|   |  $$$$$$  /$$_____/ /$$__  $$|____  $$ /$$__  $$| $$   |
|    \____  $$| $$      | $$  \__/ /$$$$$$$| $$  \ $$| $$   |
|    /$$  \ $$| $$      | $$      /$$__  $$| $$  | $$| $$   |
|   |  $$$$$$/|  $$$$$$$| $$     |  $$$$$$$| $$$$$$$/| $$   |
|    \______/  \_______/|__/      \_______/| $$____/ |__/   |
|                                          | $$             |
|                                          | $$             |
|                                          |__/             |
|                                                           |
|__________[AI powered general purpose web scraper]_________|
`
	color.Cyan(asciiart)
	fmt.Println()

	// Get user inputs
	url, prompt, useProxy, err := getUserInputs()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Create a new collector
	c := colly.NewCollector()

	// Initialize a ScrapedPage to store results
	scrapedPage := ScrapedPage{URL: url, Prompt: prompt, Proxy: useProxy}

	// Set proxy using the fetched IPPort
	if scrapedPage.Proxy {
		err := setProxy(c)
		if err != nil {
			fmt.Println("Error setting proxy:", err)
			return
		}
	}

	// called before an HTTP request is triggered
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting:", r.URL)
		scrapedPage.URL = r.URL.String() // Set the URL of the scraped page
	})

	// triggered when the scraper encounters an error
	c.OnError(func(_ *colly.Response, err error) {
		fmt.Println("Something went wrong:", err)
	})

	// Callback to collect text content, links, and images in the order they appear
	c.OnHTML("h1, h2, h3, h4, h5, h6, p, ul, ol, li, a, img", func(e *colly.HTMLElement) {
		var text string
		switch e.Name {
		case "a":
			// Collect link text and URL
			text = fmt.Sprintf("Link: %s (%s)", e.Text, e.Attr("href"))
		case "img":
			// Collect image URL
			text = fmt.Sprintf("Image: %s", e.Attr("src"))
		default:
			// Collect text content of other elements
			text = e.Text
		}
		scrapedPage.Content += text + "\n" // Append text to content
	})

	// triggered once scraping is done
	c.OnScraped(func(r *colly.Response) {
		fmt.Println("✅ website scraped!")

		// Send the scraped content to OpenAI
		response, err := sendToOpenAI(scrapedPage)
		if err != nil {
			fmt.Println("Error communicating with OpenAI:", err)
			return
		} else {

			// Write the raw JSON response to a file
			filename := "scrapi"
			err = writeJSONToFile(filename+".json", []byte(response))
			if err != nil {
				fmt.Printf("failed to write response to file: %v", err)
			}
		}

		color.Green("✅ Done!")
	})

	// Visit the target page
	c.Visit(scrapedPage.URL)
}

func getUserInputs() (string, string, bool, error) {
	// Prompt the user to enter the URL
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("🔗 URL: ")
	url, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading input:", err)
		return "", "", false, fmt.Errorf("error reading proxy input: %v", err)
	}
	url = strings.TrimSpace(url) // Trim any extra spaces or newline characters

	// Check if the URL is provided
	if url == "" {
		return "", "", false, fmt.Errorf("URL cannot be empty. Please provide a valid URL")
	}

	// Prompt the user to enter an optional prompt
	fmt.Print("🤖 Prompt (optional): ")
	prompt, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading input:", err)
		return "", "", false, fmt.Errorf("error reading proxy input: %v", err)
	}
	prompt = strings.TrimSpace(prompt) // Trim any extra spaces or newline characters

	// Set a default prompt if the user does not provide one
	if prompt == "" {
		prompt = "Here is the scraped content from a webpage:\n%s\nPlease summarize the main points."
	} else {
		prompt = fmt.Sprintf("%s\n%%s", prompt) // Format to include the scraped content
	}

	// Prompt the user to decide whether to use a proxy
	fmt.Println("🌐 Use proxy? (yes, press Enter to skip): ")
	useProxyInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading input:", err)
		return "", "", false, fmt.Errorf("error reading proxy input: %v", err)
	}
	useProxyInput = strings.TrimSpace(useProxyInput) // Trim any extra spaces or newline characters
	useProxy := strings.ToLower(useProxyInput) == "yes"

	return url, prompt, useProxy, nil

}

// setProxy fetches a proxy, sets it for the Colly collector, and prints the IP used
func setProxy(c *colly.Collector) error {
	// Fetch proxy data from the endpoint
	resp, err := http.Get("http://pubproxy.com/api/proxy?https=true&country=US,CA&type=http")
	if err != nil {
		return fmt.Errorf("failed to fetch proxy: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	var proxyResp ProxyResponse
	err = json.Unmarshal(body, &proxyResp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Check if we have at least one proxy in the response
	if len(proxyResp.Data) == 0 {
		return fmt.Errorf("no proxy found in response")
	}

	// Extract the proxy IPPort and IP address
	proxy := proxyResp.Data[0].IPPort

	// Set the proxy using the IPPort field from the response
	err = c.SetProxy("http://" + proxy)
	if err != nil {
		return fmt.Errorf("failed to set proxy: %v", err)
	}

	// Print the proxy IP being used
	fmt.Println("Proxy set to:", proxy)

	return nil
}

// sendToOpenAI sends the scraped content to OpenAI and returns the response
func sendToOpenAI(scrapedContent ScrapedPage) (string, error) {
	apiKey := goDotEnvVariable("OPENAI_API_KEY")
	apiURL := goDotEnvVariable("OPENAI_API_URL")

	// Define the request payload
	payload := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a helpful assistant that will understand the text from a website and represent it in a json format. Fullfil any commands by the user or ignore if its empty:\n" + scrapedContent.Content,
			},
			{
				"role":    "user",
				"content": scrapedContent.Prompt,
			},
		},
	}

	// Convert the payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set headers for the request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	fmt.Println("✨ Analysing with AI ...")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to OpenAI: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Check if the response status is not OK (200)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-OK response status: %d, body: %s", resp.StatusCode, body)
	}

	// Parse the response to extract the assistant's reply
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Check if choices are available in the response
	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices found in response or unexpected format: %v", result)
	}

	// Extract the content of the response
	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected format for first choice: %v", choices[0])
	}
	message, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected format for message: %v", firstChoice)
	}
	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected format for content: %v", message)
	}

	return content, nil
}

// writeJSONToFile writes the JSON response to a file
func writeJSONToFile(filename string, data []byte) error {
	// Create or overwrite the file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Write the JSON data to the file
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data to file: %v", err)
	}

	fmt.Println("Writing result to ", filename)
	return nil
}
