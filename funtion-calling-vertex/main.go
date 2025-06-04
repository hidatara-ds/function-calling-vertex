package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	genai "google.golang.org/genai"
)

// LoadEnv loads environment variables from .env file
func LoadEnv() error {
	// Try multiple possible locations for the .env file
	possiblePaths := []string{
		".env",                                   // Current directory
		"../config/.env",                         // Config directory
		filepath.Join(os.Getenv("HOME"), ".env"), // Home directory
		"d:\\SMP\\acti\\.env",                    // Specific Windows path
	}

	var loadErr error
	for _, path := range possiblePaths {
		err := godotenv.Load(path)
		if err == nil {
			log.Printf("Successfully loaded .env from: %s", path)
			return nil // Successfully loaded
		}
		loadErr = err
	}

	return fmt.Errorf("failed to load .env file: %v", loadErr)
}

// GetSystemInstruction mengembalikan instruksi sistem untuk AI berdasarkan lokasi pengguna
func GetSystemInstruction(latitude, longitude string) string {
	return fmt.Sprintf(`
        As Travel Buddy AI from Telkomsel, I answer travel-related questions based on provided videos or location, prioritizing accuracy and conciseness. I will retrieve data from Google Maps for location name, address, reviews, ratings, distance, the Google Maps link, and the profile picture of the reviewer.

        Guidelines:

        - Identify user location based on latitude: %s and longitude: %s.
        - Focus on Google Maps data if the user's question pertains to a place or location.
        - Focus on information within the video if the user's question concerns objects in the video.
        - Use the city name as the smallest scope if the user asks about nearby locations.
        - Provide suggestions for locations or venues (e.g., food, drinks, coffee, hangout spots, malls, etc.) if the user mentions them.
        - Suggest locations based on ratings, reviews, historical significance, and proximity.
        - Detect the source language (audio or text) and respond to the user in the same language.
        - Display the distance in kilometers (km) from the user's location to each suggested location.
        - Offer currency conversion to assist foreign tourists.
        - Provide current weather information or a weather forecast if user ask. If the user doesn't specify a location, the default location will be the user's current citty or location.
        - For each location suggestion, include its corresponding Google Maps link.
        - For the provided review of each location, include the URL of the profile picture of the user who wrote that review (if available).

        For inquiries regarding Halal status, the analysis will be based on visual cues (halal logos, ingredients visible in the menu/food images). Information regarding the presence of non-halal ingredients in the menu will be provided if detected.

		- IMPORTANT: If the user asks about the weather, ALWAYS call the getCurrentWeather function with the appropriate city or latitude/longitude parameters.
		- IMPORTANT: If the user asks about place recommendations, ALWAYS call the getPlaceRecommendation function with the appropriate query parameters.
		- IMPORTANT: If the user asks about currency exchange rates, ALWAYS call the getExchangeRate function with the appropriate from and to parameters.
		- IMPORTANT: If the user's query spans multiple topics (e.g. weather AND place recommendations), call the appropriate function for EACH topic in sequence.
        - IMPORTANT: After giving an answer, always end with a follow-up question in Indonesian, written in a friendly and relaxed tone that is suitable for all ages. Keep the language clear, casual, and approachable—as if you’re talking to a friend or family member. Feel free to use light expressions like “penasaran gak?”, “udah pernah coba?”, or “mau aku bantu cari lagi?”. Add a simple, warm emoji (e.g., 😊, 😄, ✨) when it fits the tone naturally.

        Answer format in JSON:
        {
            "response": "your response text",
            "transcript": "audio transcription from the video (if available)",
            "locations": [
                {
                "name": "location name",
                "rating": "location rating on Google Maps",
                "total_user_rating": "number of reviews on Google Maps",
                "address": "location address on Google Maps",
                "distance": "distance from user (km)",
                "review": "one review from Google Maps",
                "pp_reviewer": "URL of the profile picture of the reviewer (if available)",
                "gmap_link": "Google Maps link to the location"
                }
            ]
        }
        `, latitude, longitude)
}

// Fungsi untuk mencari tempat terdekat menggunakan Google Places API
func GetNearbyPlaces(location string, placeType string, radius int) error {
	apiKey := getGooglePlacesAPIKey()
	if apiKey == "" {
		fmt.Println("API key tidak ditemukan. Set GOOGLE_PLACE_API_KEY sebagai environment variable.")
		return fmt.Errorf("API key tidak ditemukan")
	}

	url := fmt.Sprintf(
		"https://maps.googleapis.com/maps/api/place/nearbysearch/json?location=%s&radius=%d&type=%s&key=%s",
		location, radius, placeType, apiKey,
	)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Gagal menghubungi Google Places API:", err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error membaca response: %v", err)
	}

	// Hapus output JSON mentah
	// fmt.Println("Status code:", resp.StatusCode)
	// fmt.Println("Raw response:", string(body))

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("error parsing JSON: %v", err)
	}

	results, ok := data["results"].([]interface{})
	if !ok || len(results) == 0 {
		fmt.Println("Tidak ditemukan tempat terdekat.")
		return nil
	}

	fmt.Println("Tempat terdekat:")
	for i, item := range results {
		if i >= 5 {
			break
		}
		place := item.(map[string]interface{})
		name := place["name"].(string)
		vicinity := place["vicinity"].(string)

		// Tambahkan rating jika tersedia
		ratingStr := ""
		if rating, hasRating := place["rating"].(float64); hasRating {
			ratingStr = fmt.Sprintf(" (Rating: %.1f)", rating)
		}

		fmt.Printf("%d. %s%s (%s)\n", i+1, name, ratingStr, vicinity)
	}
	return nil
}

// Variabel untuk konfigurasi Vertex AI
var setupPayloadVertex = `{
	"setup": {
		"model": "projects/arboreal-avatar-458607-t7/locations/us-central1/publishers/google/models/gemini-2.0-flash-exp",
		"generationConfig": {
			"responseModalities": ["TEXT"],
			"temperature": 0.7,
			"topP": 0.95,
			"topK": 40
		},
		"tools": [
			{
				"googleSearch": {}
			},
			{
				"function_declarations": [
					{
						"name": "getCurrentWeather",
						"description": "Returns the current weather base on longitude latitude and base on city.",
						"parameters": {
							"type": "object",
							"properties": {
								"latitude": {"type": "string"},
								"longitude": {"type": "string"},
								"city": {"type": "string"}
							}
						}
					},
					{
						"name": "getPlaceRecommendation",
						"description": "Returns the recommendation place in a location, ex: restaurant, hotel, etc.",
						"parameters": {
							"type": "object",
							"properties": {
								"query": {"type": "string"}
							},
							"required": ["query"]
						}
					},
					{
						"name": "getExchangeRate",
						"description": "Returns the current exchange rate from one currency to another. Use ISO 4217 currency codes (e.g., USD, IDR, EUR).",
						"parameters": {
							"type": "object",
							"properties": {
								"from": {
									"type": "string",
									"description": "The currency code to convert from (ISO 4217)."
								},
								"to": {
									"type": "string",
									"description": "The currency code to convert to (ISO 4217)."
								}
							},
							"required": ["from", "to"]
						}
					}
				]
			}
		],
		"system_instruction": {
			"role": "system",
			"parts": [{
				"text": "%s"
			}]
		}
	}
}`

func main() {
	// Load environment variables first
	if err := LoadEnv(); err != nil {
		log.Printf("Peringatan: %v", err)
		log.Println("Pastikan file .env berisi semua API key yang diperlukan:")
		log.Println("- OPEN_WEATHER_API_KEY")
		log.Println("- GOOGLE_PLACE_API_KEY")
		log.Println("- CURRENCY_API_KEY")
		log.Println("- GOOGLE_SEARCH_API_KEY")
		log.Println("- GOOGLE_SEARCH_CX")
	}

	err := generateWithFuncCall("recomendasi warung enak terdekat di sleman? dan cuaca di kota sleman saat ini?")
	if err != nil {
		log.Fatalf("Error generating with function call: %v", err)
	}
	log.Println("======================================")

	// Contoh penggunaan fitur baru: cari 5 restoran terdekat dari Tugu Jogja
	// err = GetNearbyPlaces("-7.782889,110.367083", "restaurant", 500)
	// if err != nil {
	// 	log.Fatalf("Error mencari tempat terdekat: %v", err)
	// }

	// err := generateWithFuncCall("50 EUR berapa Rupiah?")
	// if err != nil {
	// 	log.Fatalf("Error generating with function call: %v", err)
	// }

	// log.Println("======================================")

	// err := generateWithFuncCall("cuaca saat ini di Sleman(7.7325, 110.4024)?")
	// if err != nil {
	// 	log.Fatalf("Error generating with function call: %v", err)
	// }

	// log.Println("======================================")

}

// --- Konstanta API Keys (Fallbacks jika environment variables tidak ada) ---
func getOpenWeatherMapAPIKey() string {
	key := os.Getenv("OPEN_WEATHER_API_KEY")
	if key == "" {
		log.Println("WARNING: OPEN_WEATHER_API_KEY tidak ditemukan di environment variables")
		return "" // Tidak lagi menggunakan hardcoded value
	}
	return key
}

func getGooglePlacesAPIKey() string {
	key := os.Getenv("GOOGLE_PLACE_API_KEY")
	if key == "" {
		log.Println("WARNING: GOOGLE_PLACE_API_KEY tidak ditemukan di environment variables")
		return "" // Tidak lagi menggunakan hardcoded value
	}
	return key
}

func getExchangeRateAPIKey() string {
	key := os.Getenv("CURRENCY_API_KEY")
	if key == "" {
		log.Println("WARNING: CURRENCY_API_KEY tidak ditemukan di environment variables")
		return "" // Tidak lagi menggunakan hardcoded value
	}
	return key
}

func GetWeatherDataByLatLong(lat, lon string) (*genai.FunctionResponse, error) {
	apiKey := getOpenWeatherMapAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("API key untuk OpenWeatherMap tidak tersedia")
	}

	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?lat=%s&lon=%s&units=metric&appid=%s", lat, lon, apiKey)
	log.Printf("Requesting weather API: %s", url) // Tambahkan log ini

	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error membuat request: %v", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error mengirim request: %v", err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		panic(err)
	}

	return &genai.FunctionResponse{
		Name:     "getCurrentWeather", // Pastikan nama sesuai dengan deklarasi fungsi
		Response: result,
	}, nil
}

func GetWeatherDataByCity(city string) (*genai.FunctionResponse, error) {
	apiKey := getOpenWeatherMapAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("API key untuk OpenWeatherMap tidak tersedia")
	}

	params := url.Values{}
	params.Add("q", city)
	params.Add("units", "metric")
	params.Add("appid", apiKey)

	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?%s", params.Encode())
	log.Printf("Requesting weather API: %s", url) // Tambahkan log ini

	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error membuat request: %v", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error mengirim request: %v", err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		panic(err)
	}

	return &genai.FunctionResponse{
		Name:     "getPlaceRecommendation", // Nama yang benar
		Response: result,
	}, nil
}

func GetPlaceRecommendation(query string) (*genai.FunctionResponse, error) {
	apiKey := getGooglePlacesAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("API key untuk Google Places tidak tersedia")
	}

	url := fmt.Sprintf("https://places.googleapis.com/v1/places:searchText")
	log.Printf("Requesting place API: %s", url) // Tambahkan log ini

	data := map[string]string{
		"textQuery": query,
	}

	// Encode ke JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error membuat request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Goog-Api-Key", apiKey)
	req.Header.Add("X-Goog-FieldMask", "places.displayName,places.formattedAddress,places.priceLevel")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error mengirim request: %v", err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		panic(err)
	}

	return &genai.FunctionResponse{
		Name:     "getCurrentWeather",
		Response: result,
	}, nil
}

func GetExchangeRate(dari, ke string) (*genai.FunctionResponse, error) {
	apiKey := getExchangeRateAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("API key untuk Exchange Rate tidak tersedia")
	}

	baseURL := "https://api.exchangerate.host/convert"
	params := url.Values{}
	params.Add("access_key", apiKey)
	params.Add("from", strings.ToUpper(dari))
	params.Add("to", strings.ToUpper(ke))
	params.Add("amount", "1") // Ambil rate untuk 1 unit

	finalURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	log.Printf("Requesting exchange rate API: %s", finalURL)

	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, finalURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error membuat request: %v", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error mengirim request: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error membaca body response: %v", err)
	}
	// Tambahkan log untuk body response
	log.Printf("Exchange rate API response body: %s", string(body))

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API mengembalikan status error: %s, Body: %s", res.Status, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		// Jika unmarshal gagal, coba log body lagi untuk debug
		log.Printf("Error unmarshal response body: %s", string(body))
		return nil, fmt.Errorf("error unmarshal response: %v", err)
	}
	// Validasi success flag
	success, ok := result["success"].(bool)
	if !ok || !success {
		// Coba ekstrak pesan error jika ada
		errorInfo, hasError := result["error"].(map[string]any)
		if hasError {
			errorType, _ := errorInfo["type"].(string)
			errorMsg, _ := errorInfo["info"].(string)
			return nil, fmt.Errorf("API call tidak berhasil (success=false), error type: %s, info: %s", errorType, errorMsg)
		}
		return nil, fmt.Errorf("API call tidak berhasil (success=false), response: %v", result)
	}
	// Validasi quote hasil (gunakan "info" dan "quote" sesuai struktur API)
	info, ok := result["info"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("format data info tidak sesuai dalam response: %v", result)
	}

	quote, ok := info["quote"].(float64) // Gunakan "quote" bukan "rate"
	if !ok {
		// Coba cek apakah quote adalah integer
		quoteInt, okInt := info["quote"].(int)
		if okInt {
			quote = float64(quoteInt)
			ok = true
		}
	}

	if !ok || quote == 0 {
		return nil, fmt.Errorf("rate dari %s ke %s tidak valid atau 0 dari API. Info: %v", dari, ke, info)
	}

	// Kembalikan hanya nilai quote sebagai response
	return &genai.FunctionResponse{
		Name:     "getExchangeRate",             // Pastikan nama sesuai dengan deklarasi fungsi
		Response: map[string]any{"rate": quote}, // Kembalikan map dengan key "rate"
	}, nil
}

func generateWithFuncCall(question string) error {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		HTTPOptions: genai.HTTPOptions{APIVersion: "v1"},
	})
	if err != nil {
		return fmt.Errorf("failed to create genai client: %w", err)
	}

	// Tambahkan log untuk system instruction
	systemInstruction := GetSystemInstruction("-7.7325", "110.4024") // Koordinat default untuk Sleman
	// Hapus atau komentari baris ini
	// log.Printf("System Instruction yang digunakan: %s", systemInstruction)

	// Format setupPayloadVertex dengan system instruction
	formattedSetupPayload := fmt.Sprintf(setupPayloadVertex, systemInstruction)

	// Tambahkan log yang lebih ringkas
	log.Printf("Mengirim permintaan ke Vertex AI dengan koordinat: %s, %s", "-7.7325", "110.4024")
	log.Printf("setupPayloadVertex yang dikirim ke Vertex AI: %s", formattedSetupPayload)

	weatherFunc := &genai.FunctionDeclaration{
		Description: "Returns the current weather base on longitude latitude and base on city.",
		Name:        "getCurrentWeather", // Nama harus cocok dengan FunctionResponse.Name
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"latitude":  {Type: "string"},
				"longitude": {Type: "string"},
				"city":      {Type: "string"},
			},
			Required: []string{},
		},
	}

	placeFunc := &genai.FunctionDeclaration{
		Description: "Returns the recommendation place in a location, ex: restaurant, hotel, etc.",
		Name:        "getPlaceRecommendation", // Nama harus cocok dengan FunctionResponse.Name
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"query": {Type: "string"},
			},
			Required: []string{"query"},
		},
	}

	exchangeRateFunc := &genai.FunctionDeclaration{
		Description: "Returns the current exchange rate from one currency to another. Use ISO 4217 currency codes (e.g., USD, IDR, EUR).",
		Name:        "getExchangeRate", // Nama harus cocok dengan FunctionResponse.Name
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"from": {
					Type:        "string",
					Description: "The currency code to convert from (ISO 4217).",
				},
				"to": {
					Type:        "string",
					Description: "The currency code to convert to (ISO 4217).",
				},
			},
			Required: []string{"from", "to"},
		},
	}

	googleSearchFunc := &genai.FunctionDeclaration{
		Description: "Search for information on the web using Google Search API.",
		Name:        "googleSearch", // Nama harus cocok dengan FunctionResponse.Name
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"query": {
					Type:        "string",
					Description: "The search query to look up on Google.",
				},
			},
			Required: []string{"query"},
		},
	}

	config := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{
			{
				FunctionDeclarations: []*genai.FunctionDeclaration{
					weatherFunc,
					placeFunc,
					exchangeRateFunc,
					googleSearchFunc,
				},
			},
		},
		Temperature: genai.Ptr(float32(0.0)),
	}

	modelName := "gemini-2.0-flash-001"

	// Inisialisasi riwayat percakapan dengan pertanyaan pengguna
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: question},
			},
		},
	}

	// Tambahkan system instruction ke konfigurasi
	config.SystemInstruction = &genai.Content{
		Role: "system",
		Parts: []*genai.Part{
			{Text: systemInstruction},
		},
	}

	// Variabel untuk melacak apakah kita perlu melakukan iterasi lagi
	needsAnotherIteration := true
	maxIterations := 3 // Batasi jumlah iterasi untuk menghindari loop tak terbatas
	iteration := 0

	// Variabel untuk menyimpan respons akhir
	var finalResponse strings.Builder

	for needsAnotherIteration && iteration < maxIterations {
		iteration++
		log.Printf("Iterasi #%d untuk pertanyaan: %s", iteration, question)

		resp, err := client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			return fmt.Errorf("failed to generate content (iterasi #%d): %w", iteration, err)
		}

		// Handle potential lack of candidates (e.g., safety blocking)
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
			log.Println("Model did not return any content or function call.")

			if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "" {
				log.Printf("Finish Reason: %s", resp.Candidates[0].FinishReason)
				if resp.Candidates[0].SafetyRatings != nil {
					log.Printf("Safety Ratings: %+v", resp.Candidates[0].SafetyRatings)
				}
			}
			break // Keluar dari loop jika tidak ada respons
		}

		var funcCall *genai.FunctionCall
		var textResponse string

		// Periksa apakah ada function call atau respons teks
		for _, p := range resp.Candidates[0].Content.Parts {
			if p.FunctionCall != nil {
				funcCall = p.FunctionCall
				log.Printf(">> The model suggests to call the function ")
				log.Printf("%q with args: %v\n", funcCall.Name, funcCall.Args)
				break // Assuming only one function call per response
			} else if p.Text != "" {
				textResponse += p.Text
			}
		}

		if funcCall == nil {
			// Jika tidak ada function call, tambahkan respons teks ke respons akhir
			if textResponse != "" {
				finalResponse.WriteString(textResponse)
				finalResponse.WriteString("\n\n")
			}
			// Tidak perlu iterasi lagi
			needsAnotherIteration = false
		} else {
			// Jika ada function call, eksekusi
			var funcResp *genai.FunctionResponse
			var callErr error

			// --- Refactored Function Call Handling ---
			switch funcCall.Name {
			case "getCurrentWeather":
				if lat, latOK := funcCall.Args["latitude"].(string); latOK {
					if lon, lonOK := funcCall.Args["longitude"].(string); lonOK {
						funcResp, callErr = GetWeatherDataByLatLong(lat, lon)
					} else {
						callErr = fmt.Errorf("invalid function call arguments for getCurrentWeather: 'longitude' missing or not a string")
					}
				} else if city, cityOK := funcCall.Args["city"].(string); cityOK {
					funcResp, callErr = GetWeatherDataByCity(city)
				} else {
					callErr = fmt.Errorf("invalid function call arguments for getCurrentWeather: requires either (latitude, longitude) or city")
				}

			case "getPlaceRecommendation":
				if query, ok := funcCall.Args["query"].(string); ok {
					funcResp, callErr = GetPlaceRecommendation(query)
				} else {
					callErr = fmt.Errorf("invalid function call arguments for getPlaceRecommendation: 'query' missing or not a string")
				}

			case "getExchangeRate":
				var from, to string
				var fromOK, toOK bool

				if fromArg, ok := funcCall.Args["from"].(string); ok {
					from = fromArg
					fromOK = true
				}
				if toArg, ok := funcCall.Args["to"].(string); ok {
					to = toArg
					toOK = true
				}

				if fromOK && toOK {
					funcResp, callErr = GetExchangeRate(from, to)
				} else {
					callErr = fmt.Errorf("invalid function call arguments for getExchangeRate: requires 'from' and 'to' strings")
				}

			default:
				callErr = fmt.Errorf("unknown function call: %s", funcCall.Name)
			}

			// --- Common Error Handling and Response Sending ---
			if callErr != nil {
				// If the function call itself failed, create an error response for the model
				log.Printf("Error calling function %s: %v", funcCall.Name, callErr)
				funcResp = &genai.FunctionResponse{
					Name: funcCall.Name, // Echo the function name back
					Response: map[string]any{
						"error": fmt.Sprintf("Failed to execute function %s: %v", funcCall.Name, callErr),
					},
				}
				// Reset callErr as we are handling it by sending the error response back to the model
				callErr = nil
			}

			// Tambahkan hasil function call ke riwayat percakapan
			contents = append(contents,
				&genai.Content{ // Model's request to call the function
					Role: genai.RoleModel,
					Parts: []*genai.Part{
						{FunctionCall: funcCall},
					},
				},
				&genai.Content{ // The result of the function call
					Role: "function", // Role must be "function" for FunctionResponse
					Parts: []*genai.Part{
						{FunctionResponse: funcResp},
					},
				},
			)

			// Tambahkan prompt untuk meminta model melanjutkan dengan pertanyaan lain jika ada
			if iteration == 1 {
				// Pada iterasi pertama, minta model untuk memeriksa apakah ada pertanyaan lain yang perlu dijawab
				contents = append(contents, &genai.Content{
					Role: "user",
					Parts: []*genai.Part{
						{Text: "Terima kasih atas informasi tersebut. Apakah ada bagian lain dari pertanyaan saya yang belum terjawab? Jika ya, tolong jawab bagian tersebut."},
					},
				})
			} else {
				// Pada iterasi berikutnya, minta model untuk memberikan ringkasan akhir
				contents = append(contents, &genai.Content{
					Role: "user",
					Parts: []*genai.Part{
						{Text: "Terima kasih. Sekarang berikan saya ringkasan lengkap dari semua informasi yang telah Anda kumpulkan."},
					},
				})
				// Ini adalah iterasi terakhir
				needsAnotherIteration = false
			}
		}
	}

	// Jika kita keluar dari loop tanpa respons akhir, tampilkan respons terakhir dari model
	if finalResponse.Len() == 0 && len(contents) > 1 {
		// Ambil respons terakhir dari model
		resp, err := client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			return fmt.Errorf("failed to generate final content: %w", err)
		}

		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, p := range resp.Candidates[0].Content.Parts {
				if p.Text != "" {
					finalResponse.WriteString(p.Text)
				}
			}
		}
	}

	// Blok berikut ini dihapus karena fungsionalitasnya sudah diintegrasikan
	// ke dalam SystemInstruction untuk diproses dalam panggilan API utama.
	/*
		// Tambahkan rekomendasi pertanyaan lanjutan
		if finalResponse.Len() > 0 {
			// Buat prompt untuk meminta rekomendasi pertanyaan lanjutan
			suggestionsPrompt := &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{Text: "Based on previous conversations about {topic}, are you also interested in exploring other related topics, such as {related_topics}? answer in indonesian language."},
				},
			}

			// Buat salinan contents untuk mendapatkan rekomendasi
			suggestionContents := make([]*genai.Content, len(contents))
			copy(suggestionContents, contents)
			suggestionContents = append(suggestionContents, suggestionsPrompt)

			// Konfigurasi tanpa function calls untuk rekomendasi
			suggestionConfig := &genai.GenerateContentConfig{
				Temperature: genai.Ptr(float32(0.7)), // Sedikit lebih kreatif untuk rekomendasi
			}

			// Dapatkan rekomendasi
			suggResp, err := client.Models.GenerateContent(ctx, modelName, suggestionContents, suggestionConfig)
			if err == nil && len(suggResp.Candidates) > 0 && suggResp.Candidates[0].Content != nil {
				finalResponse.WriteString("\n                                                 \n")
				for _, p := range suggResp.Candidates[0].Content.Parts {
					if p.Text != "" {
						finalResponse.WriteString(p.Text)
					}
				}
			}
		}
	*/

	// Tampilkan respons akhir
	if finalResponse.Len() > 0 {
		fmt.Println(finalResponse.String())
	} else {
		fmt.Println("Maaf, saya tidak dapat menjawab pertanyaan Anda saat ini.")
	}

	return nil
}
