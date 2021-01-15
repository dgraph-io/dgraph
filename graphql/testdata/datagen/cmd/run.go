package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	loremIpsum = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, " +
		"sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. "

	graphqlReqBody = `{
		"query": "%s",
		"variables": {"input": %s}
	}`
	addCountryMutation = `mutation ($input: [AddCountryInput!]!){
			addCountry(input: $input) {
				numUids
			}
		}`
	addRestaurantMutation = `mutation ($input: AddRestaurantInput!){
			addRestaurant(input: [$input]) {
				numUids
			}
		}`
)

type country struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}
type city struct {
	Id      string  `json:"id,omitempty"`
	Name    string  `json:"name,omitempty"`
	Country country `json:"country,omitempty"`
}
type address struct {
	Lat      float64 `json:"lat"`
	Long     float64 `json:"long"`
	Address  string  `json:"address,omitempty"`
	Locality string  `json:"locality,omitempty"`
	City     city    `json:"city,omitempty"`
	Zipcode  int     `json:"zipcode,omitempty"`
}
type cuisine struct {
	Name string `json:"name,omitempty"`
}
type dish struct {
	Name        string  `json:"name,omitempty"`
	Pic         string  `json:"pic,omitempty"`
	Price       float64 `json:"price,omitempty"`
	Description string  `json:"description,omitempty"`
	IsVeg       bool    `json:"isVeg"`
	Cuisine     cuisine `json:"cuisine,omitempty"`
}
type restaurant struct {
	Xid       string    `json:"xid,omitempty"`
	Name      string    `json:"name,omitempty"`
	Pic       string    `json:"pic,omitempty"`
	Addr      address   `json:"addr,omitempty"`
	Rating    float64   `json:"rating,omitempty"`
	CostFor2  float64   `json:"costFor2,omitempty"`
	Currency  string    `json:"currency,omitempty"`
	Cuisines  []cuisine `json:"cuisines,omitempty"`
	Dishes    []dish    `json:"dishes,omitempty"`
	CreatedAt string    `json:"createdAt,omitempty"`
}

type gqlResp struct {
	Data struct {
		AddRestaurant struct {
			NumUids int
		}
		AddCountry struct {
			NumUids int
		}
	}
	Errors []interface{}
}

type dataFile []struct {
	Restaurants []struct {
		Restaurant struct {
			UserRating struct {
				AggregateRating string `json:"aggregate_rating"`
			} `json:"user_rating"`
			Name              string
			AverageCostForTwo int `json:"average_cost_for_two"`
			Cuisines          string
			Location          struct {
				Latitude  string
				Address   string
				City      string
				CountryId int `json:"country_id"`
				CityId    int `json:"city_id"`
				Zipcode   string
				Longitude string
				Locality  string
			}
			Currency string
			Id       string
			Thumb    string
		}
	}
}

func createAndAddRestaurants(data dataFile) (int, int, int) {
	invalid := 0
	added := 0
	failed := 0
	for _, datum := range data {
		for _, obj := range datum.Restaurants {
			rest := obj.Restaurant
			loc := rest.Location
			lat, _ := strconv.ParseFloat(loc.Latitude, 64)
			long, _ := strconv.ParseFloat(loc.Longitude, 64)
			zip, _ := strconv.Atoi(loc.Zipcode)
			rating, _ := strconv.ParseFloat(rest.UserRating.AggregateRating, 64)
			cuisineNames := strings.Split(rest.Cuisines, ", ")
			cuisines := make([]cuisine, 0, len(cuisineNames))
			for _, cuisineName := range cuisineNames {
				if strings.TrimSpace(cuisineName) != "" {
					cuisines = append(cuisines, cuisine{Name: cuisineName})
				}
			}
			now := time.Now()
			restaurant := restaurant{
				Xid:  rest.Id,
				Name: rest.Name,
				Pic:  rest.Thumb,
				Addr: address{
					Lat:      lat,
					Long:     long,
					Address:  loc.Address,
					Locality: loc.Locality,
					City: city{
						Id:      strconv.Itoa(loc.CityId),
						Name:    loc.City,
						Country: country{Id: strconv.Itoa(loc.CountryId)},
					},
					Zipcode: zip,
				},
				Rating:    rating,
				CostFor2:  float64(rest.AverageCostForTwo),
				Currency:  rest.Currency,
				Cuisines:  cuisines,
				CreatedAt: now.Format("2006-01-02") + "T" + now.Format("15:04:05"),
			}
			if !isValidRestaurant(&restaurant) {
				invalid++
				continue
			}
			restaurant.Dishes = generateDishes(&restaurant)
			success := addRestaurant(restaurant)
			if success {
				added++
			} else {
				failed++
			}
			fmt.Println("added: ", added, "failed: ", failed, "invalid: ", invalid)
		}
	}
	log.Println("Total invalid Restaurants: ", invalid)
	log.Println("Total added Restaurants: ", added)
	log.Println("Total failed Restaurants: ", failed)
	return invalid, added, failed
}

func generateDishes(rest *restaurant) []dish {
	numCuisines := len(rest.Cuisines)
	if numCuisines == 0 || rest.CostFor2 <= 0 {
		return nil
	}

	rand.Seed(time.Now().UnixNano())
	numDishes := 1 + rand.Intn(viper.GetInt(maxDishes4AnyRestaurant))
	dishes := make([]dish, 0, numDishes)
	// Although, costFor2 is cost for 2 people, but here we will consider it as cost of two dishes
	// to generate dish prices. Multiplying by 100, so will divide by 100 later to make it look
	// like an actual price (a floating point with max two decimals).
	priceToDivide := int(rest.CostFor2) * numDishes / 2 * 100
	dishPrices := make([]int, 0, numDishes)

	for i := 0; i < numDishes-1; i++ {
		dishPrices = append(dishPrices, rand.Intn(priceToDivide))
	}
	dishPrices = append(dishPrices, priceToDivide)
	sort.Ints(dishPrices)
	// [a, b, c, max]
	// a + (b - a) + (c - b) + (max - c) = max
	for i := numDishes - 1; i > 0; i-- {
		dishPrices[i] = dishPrices[i] - dishPrices[i-1]
		// keep it min 1
		if dishPrices[i] == 0 {
			dishPrices[i] = 1
		}
	}
	if dishPrices[0] == 0 {
		dishPrices[0] = 1
	}

	for i := 0; i < numDishes; i++ {
		dish := dish{
			Name:        "Dish " + strconv.Itoa(i+1),
			Pic:         rest.Pic,
			Price:       float64(dishPrices[i]) / 100,
			Description: loremIpsum,
			IsVeg:       rand.Intn(2) == 0,
			Cuisine:     rest.Cuisines[rand.Intn(numCuisines)],
		}
		dishes = append(dishes, dish)
	}

	return dishes
}

func readRestaurantData() (dataFile, error) {
	restaurantDataFile := viper.GetString(restaurantDataFilePath)
	if !filepath.IsAbs(restaurantDataFile) {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			return nil, err
		}
		restaurantDataFile = filepath.Join(dir, restaurantDataFile)
	}

	b, err := ioutil.ReadFile(restaurantDataFile)
	if err != nil {
		return nil, err
	}

	var data dataFile
	err = json.Unmarshal(b, &data)

	return data, err
}

var errCount = 0

func reportError(count int) {
	errCount += count
	if errCount >= viper.GetInt(maxErrorsInRestaurantAddition) {
		log.Fatal("Errored too many times while adding restaurants, exiting.")
	}
}

func isValidRestaurant(rest *restaurant) bool {
	if strings.TrimSpace(rest.Name) == "" || strings.TrimSpace(rest.Xid) == "" || strings.
		TrimSpace(rest.CreatedAt) == "" || strings.TrimSpace(rest.Addr.
		Locality) == "" || strings.TrimSpace(rest.Addr.Address) == "" || strings.TrimSpace(
		rest.Addr.City.Id) == "" || strings.TrimSpace(rest.Addr.City.
		Name) == "" || strings.TrimSpace(rest.Addr.City.Country.Id) == "" {
		return false
	}
	return true
}

func addRestaurant(rest restaurant) bool {
	resp, err := makeGqlReq(addRestaurantMutation, rest)
	if err != nil {
		log.Println("Error while adding restaurant id: ", rest.Xid, ", err: ", err)
		reportError(1)
		return false
	}

	if len(resp.Errors) != 0 {
		log.Println("Error while adding restaurant id: ", rest.Xid, ", err: ", resp.Errors)
		reportError(len(resp.Errors))
		return false
	}
	if resp.Data.AddRestaurant.NumUids <= 0 {
		log.Println("Unable to add restaurant id: ", rest.Xid, ", got numUids: ",
			resp.Data.AddRestaurant.NumUids)
		reportError(1)
		return false
	}

	log.Println("Added restaurant id: ", rest.Xid, " with dishes: ", len(rest.Dishes))
	return true
}

func addCountries() {
	resp, err := makeGqlReq(addCountryMutation, countries)
	if err != nil {
		log.Println("Error while adding countries: ", err)
		return
	}

	if len(resp.Errors) != 0 {
		log.Println("Error while adding countries: ", resp.Errors)
	}
	log.Println("Added countries: ", resp.Data.AddCountry.NumUids)
}

var countries = []country{
	{Id: "1", Name: "India"},
	{Id: "14", Name: "Australia"},
	{Id: "30", Name: "Brazil"},
	{Id: "37", Name: "Canada"},
	{Id: "94", Name: "Indonesia"},
	{Id: "148", Name: "New Zealand"},
	{Id: "162", Name: "Phillipines"},
	{Id: "166", Name: "Qatar"},
	{Id: "184", Name: "Singapore"},
	{Id: "189", Name: "South Africa"},
	{Id: "191", Name: "Sri Lanka"},
	{Id: "208", Name: "Turkey"},
	{Id: "214", Name: "UAE"},
	{Id: "215", Name: "United Kingdom"},
	{Id: "216", Name: "United States"},
}

type GraphQLParams struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func makeGqlReq(query string, vars interface{}) (*gqlResp, error) {
	params := GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"input": vars},
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	//fmt.Println()
	//fmt.Println(string(b))
	req, err := http.NewRequest(http.MethodPost, viper.GetString(graphqlServerUrl),
		bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(viper.GetString(authorizationHeader), viper.GetString(jwt))
	httpClient := http.Client{Timeout: viper.GetDuration(httpTimeout) * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var gqlResp gqlResp
	err = json.Unmarshal(b, &gqlResp)

	return &gqlResp, err
}
