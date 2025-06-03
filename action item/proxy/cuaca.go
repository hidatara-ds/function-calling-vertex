package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Struct response dari weatherapi.com
type WeatherResponse struct {
	Location struct {
		Name      string `json:"name"`
		Region    string `json:"region"`
		Country   string `json:"country"`
		Localtime string `json:"localtime"`
	} `json:"location"`
	Current struct {
		TempC     float64 `json:"temp_c"`
		Condition struct {
			Text string `json:"text"`
			Icon string `json:"icon"`
		} `json:"condition"`
		Humidity int     `json:"humidity"`
		WindKph  float64 `json:"wind_kph"`
	} `json:"current"`
}

func cuaca() {
	http.HandleFunc("/weather", weatherHandler)

	port := "8082"
	fmt.Printf("Server running at http://localhost:%s/\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	city := r.URL.Query().Get("city")
	if city == "" {
		http.Error(w, "Parameter 'city' harus diisi", http.StatusBadRequest)
		return
	}

	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		http.Error(w, "API key tidak ditemukan di environment variable", http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("http://api.weatherapi.com/v1/current.json?key=%s&q=%s", apiKey, city)

	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, "Gagal mengakses weather API", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Gagal mendapatkan data cuaca dari API", http.StatusInternalServerError)
		return
	}

	var weather WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		http.Error(w, "Gagal memproses data cuaca", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weather)
}
