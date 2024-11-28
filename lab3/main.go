package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	geocodeAPIKey = "99899389-b823-4cc4-8168-3685209df151"
	weatherAPIKey = "4a135e8f34c89357f333954020b0af69"
	placeAPIKey   = "5ae2e3f221c38a28845f05b68f815e923dfa0209471cf50b89c659c3"
)

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Locations struct {
	Name  string `json:"name"`
	Point Point  `json:"point"`
}

type GeocodeResult struct {
	Locations []Locations `json:"hits"`
}

type WeatherResult struct {
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
	Main struct {
		Temp     float64 `json:"temp"`
		Humidity int     `json:"humidity"`
	} `json:"main"`
}

type OpenTripMapPlace struct {
	XID   string  `json:"xid"`
	Name  string  `json:"name"`
	Kinds string  `json:"kinds"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

func getPlaces(lat, lng float64, radius int) ([]OpenTripMapPlace, error) {
	baseURL := "https://api.opentripmap.com/0.1/en/places/radius"
	params := url.Values{}
	params.Add("radius", fmt.Sprintf("%d", radius))
	params.Add("lon", fmt.Sprintf("%f", lng))
	params.Add("lat", fmt.Sprintf("%f", lat))
	params.Add("apikey", placeAPIKey)
	params.Add("limit", "10")

	resp, err := http.Get(fmt.Sprintf("%s?%s", baseURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка API OpenTripMap: %s", resp.Status)
	}

	// Парсинг ответа
	var rawResponse struct {
		Features []struct {
			Properties struct {
				XID   string `json:"xid"`
				Name  string `json:"name"`
				Kinds string `json:"kinds"`
			} `json:"properties"`
			Geometry struct {
				Coordinates [2]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, err
	}

	// Преобразование данных в наш формат
	places := make([]OpenTripMapPlace, 0, len(rawResponse.Features))
	for _, feature := range rawResponse.Features {
		places = append(places, OpenTripMapPlace{
			XID:   feature.Properties.XID,
			Name:  feature.Properties.Name,
			Kinds: feature.Properties.Kinds,
			Lat:   feature.Geometry.Coordinates[1],
			Lon:   feature.Geometry.Coordinates[0],
		})
	}

	return places, nil
}

type OpenTripMapPlaceDetails struct {
	XID     string `json:"xid"`
	Name    string `json:"name"`
	Address struct {
		City        string `json:"city"`
		State       string `json:"state"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	} `json:"address"`
	Kinds     string `json:"kinds"`
	Wikipedia string `json:"wikipedia"`
	OTM       string `json:"otm"`
	Image     string `json:"image"`
	Extracts  struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"wikipedia_extracts"`
	Lat float64 `json:"point.lat"`
	Lon float64 `json:"point.lon"`
}

func getPlaceDetails(xid string) (*OpenTripMapPlaceDetails, error) {
	baseURL := fmt.Sprintf("https://api.opentripmap.com/0.1/en/places/xid/%s", xid)
	params := url.Values{}
	params.Add("apikey", placeAPIKey)

	resp, err := http.Get(fmt.Sprintf("%s?%s", baseURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка API OpenTripMap: %s", resp.Status)
	}

	// Парсинг ответа
	var details OpenTripMapPlaceDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}

	return &details, nil
}

func getGeocode(query string, apiKey string) (GeocodeResult, error) {
	// Собираем URL
	baseURL := "https://graphhopper.com/api/1/geocode"
	params := url.Values{}
	params.Add("q", query)
	params.Add("key", apiKey)
	var result GeocodeResult
	resp, err := http.Get(fmt.Sprintf("%s?%s", baseURL, params.Encode()))
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return result, err
	}

	return result, nil
}

func getWeather(lat, lng float64, apiKey string) (*WeatherResult, error) {
	// Собираем URL
	baseURL := "https://api.openweathermap.org/data/2.5/weather"
	params := url.Values{}
	params.Add("lat", fmt.Sprintf("%f", lat))
	params.Add("lon", fmt.Sprintf("%f", lng))
	params.Add("appid", apiKey)
	params.Add("units", "metric") // Для удобства: температура в градусах Цельсия

	resp, err := http.Get(fmt.Sprintf("%s?%s", baseURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result WeatherResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func main() {
	var wg sync.WaitGroup

	fmt.Println("Введите название локации:")
	inputReader := bufio.NewReader(os.Stdin)
	query, _ := inputReader.ReadString('\n')
	query = strings.TrimSpace(query)

	wg.Add(1)
	go func() {
		defer wg.Done()
		locations, err := getGeocode(query, geocodeAPIKey)
		if err != nil {
			fmt.Println("Ошибка геокодинга:", err)
			return
		}

		if len(locations.Locations) == 0 {
			fmt.Println("Локации не найдены.")
			return
		}

		fmt.Println("Найденные локации:")
		for i, loc := range locations.Locations {
			fmt.Printf("%d. %s (%f, %f)\n", i+1, loc.Name, loc.Point.Lat, loc.Point.Lng)
		}

		fmt.Println("Введите номер локации для выбора:")
		var index int
		_, err = fmt.Scan(&index)
		if err != nil || index < 1 || index > len(locations.Locations) {
			fmt.Println("Некорректный выбор.")
			return
		}
		selectedLocation := locations.Locations[index-1]

		wg.Add(1)
		go func() {
			defer wg.Done()
			weather, err := getWeather(selectedLocation.Point.Lat, selectedLocation.Point.Lng, weatherAPIKey)
			if err != nil {
				fmt.Println("Ошибка получения погоды:", err)
				return
			}

			fmt.Printf("Погода в %s:\n", selectedLocation.Name)
			fmt.Printf("Температура: %.2f°C\n", weather.Main.Temp)
			fmt.Printf("Влажность: %d%%\n", weather.Main.Humidity)
			fmt.Printf("Описание: %s\n", weather.Weather[0].Description)
		}()

		wg.Add(1)
		go func() {

			defer wg.Done()

			places, err := getPlaces(selectedLocation.Point.Lat, selectedLocation.Point.Lng, 5000)
			if err != nil {
				fmt.Println("Ошибка при поиске мест:", err)
				return
			}

			fmt.Println("Интересные места:")
			for i, place := range places {
				fmt.Printf("%d. %s (%s)\n", i+1, place.Name, place.Kinds)

				details, err := getPlaceDetails(place.XID)
				if err != nil {
					fmt.Printf("Ошибка получения описания для %s: %v\n", place.Name, err)
					continue
				}

				fmt.Printf("Название: %s\nОписание: %s\nСсылка на Википедию: %s\n",
					details.Name, details.Extracts.Text, details.Wikipedia)
			}

		}()
	}()

	wg.Wait()
	fmt.Println("Программа завершена.")
}
