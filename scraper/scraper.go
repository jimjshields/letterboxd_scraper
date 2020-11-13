package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs/v2"
	"github.com/go-redis/redis/v8"
	"github.com/gocolly/colly/v2"
	"letterboxd_scraper/cache"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DirectorsKey string = "directors"

func ScrapeDirector(ctx context.Context, name string, streamingServices []string) ([]FilmPrices, PriceDetails) {
	url := getDirectorUrl(name)
	filmElements := scrapeFilms(url)
	films, priceDetails := scrapePrices(ctx, filmElements, streamingServices)

	// Assume that if there are films, it's a real director we can save for later
	if len(films) > 0 {
		cacheDirector(ctx, name)
	}
	return films, priceDetails
}

func cacheDirector(ctx context.Context, name string) {
	client := cache.RedisClient()
	client.SAdd(ctx, DirectorsKey, strings.Title(name))
	defer client.Close()
}

type Film struct {
	Slug        string
	Url         string
	Id          string
	ServicesUrl string
	Name        string
	Year        int
}

type PriceEntry struct {
	ServiceName string
	Format      string
	Price       float64
	PriceType   string
	FilmName    string
	FilmId      string
	Url         string
	Year        int
}

type FilmPrices struct {
	FilmName       string
	PriceEntries   []PriceEntry
	FilmDetails    Film
	Streaming      []PriceEntry
	CheapestRental PriceEntry
}

type PriceDetails struct {
	NumFilms     int
	OverallPrice string
	PricePerFilm string
}

func scrapePrices(ctx context.Context, elements []*colly.HTMLElement, streamingServices []string) ([]FilmPrices, PriceDetails) {
	var films []FilmPrices
	var servicesDataChan = make(chan FilmPrices, len(elements))

	// Use the worker pool pattern, see https://gobyexample.com/worker-pools
	for _, element := range elements {
		// scrapeFilm(c, film.Url)
		go scrapeServices(ctx, element, servicesDataChan)
	}
	for i := 0; i < len(elements); i++ {
		film := <-servicesDataChan
		film = film.getBestPrices(streamingServices)
		films = append(films, film)
	}
	sortFilmsByYear(films)
	priceDetails := calculateTotals(films)
	return films, priceDetails
}

func calculateTotals(films []FilmPrices) PriceDetails {
	var price float64
	var numFilms int
	for _, film := range films {
		if len(film.Streaming) == 0 {
			price += film.CheapestRental.Price
		}
		if len(film.Streaming) > 0 || film.CheapestRental.ServiceName != "" {
			numFilms += 1
		}
	}
	return PriceDetails{
		NumFilms:     numFilms,
		OverallPrice: fmt.Sprintf("$%.2f", price),
		PricePerFilm: fmt.Sprintf("$%.2f", price/float64(numFilms)),
	}
}

func (film FilmPrices) getBestPrices(streamingServices []string) FilmPrices {
	streamingSites := filterPrices(film.PriceEntries, filterStreamingServices(streamingServices))
	rentalPrices := filterPrices(film.PriceEntries, func(price PriceEntry) bool {
		return price.PriceType == "rent"
	})
	sortPrices(rentalPrices)
	if len(streamingSites) > 0 {
		film.Streaming = streamingSites
	} else if len(rentalPrices) > 0 {
		film.CheapestRental = rentalPrices[0]
	}
	return film
}

func sortPrices(prices []PriceEntry) {
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Price < prices[j].Price
	})
}

func sortFilmsByYear(films []FilmPrices) {
	sort.Slice(films, func(i, j int) bool {
		return films[i].FilmDetails.Year < films[j].FilmDetails.Year
	})
}

func filterPrices(slice []PriceEntry, filterExpr func(entry PriceEntry) bool) []PriceEntry {
	filteredSlice := make([]PriceEntry, 0)
	for _, item := range slice {
		if filterExpr(item) {
			filteredSlice = append(filteredSlice, item)
		}
	}
	return filteredSlice
}

func filterStreamingServices(streamingServices []string) func(price PriceEntry) bool {
	return func(price PriceEntry) bool {
		isStreaming := price.PriceType == "streaming"
		if isStreaming {
			for _, service := range streamingServices {
				if price.ServiceName == service {
					return true
				}
			}
		}
		return false
	}
}

func scrapeFilms(url string) []*colly.HTMLElement {
	c := colly.NewCollector()
	elements := make([]*colly.HTMLElement, 0)
	c.OnHTML("li.poster-container > div", func(e *colly.HTMLElement) {
		elements = append(elements, e)
	})
	err := c.Visit(url)
	if err != nil {
		fmt.Println(err)
	}
	c.Wait()
	return elements
}

func parseJson(e *colly.Response) *gabs.Container {
	jsonParsed, err := gabs.ParseJSON(e.Body)
	if err != nil {
		panic(err)
	}
	return jsonParsed
}

func getServicesData(jsonParsed *gabs.Container, film Film) FilmPrices {
	rent := jsonParsed.Path("best.rent")
	streaming := jsonParsed.Path("best.stream")
	prices := make([]PriceEntry, 0)
	for _, price := range getPrices(film, "streaming", streaming) {
		prices = append(prices, price)
	}
	for _, price := range getPrices(film, "rent", rent) {
		prices = append(prices, price)
	}
	filmPriceEntry := FilmPrices{FilmName: film.Name, PriceEntries: prices, FilmDetails: film}
	return filmPriceEntry
}

func getPrices(film Film, priceType string, prices *gabs.Container) []PriceEntry {
	priceData := make([]PriceEntry, 0)
	for _, child := range prices.Children() {
		price := getPrice(child.Path("price").String())
		priceData = append(priceData, PriceEntry{
			ServiceName: strings.Trim(child.Path("name").String(), "\""),
			Format:      strings.Trim(child.Path("format").String(), "\""),
			Price:       price,
			PriceType:   priceType,
			FilmName:    film.Name,
			FilmId:      film.Id,
			Url:         film.ServicesUrl,
			Year:        film.Year,
		})
	}
	return priceData
}

var priceRegex = regexp.MustCompile(`\d+\.\d+`)

func getPrice(priceString string) float64 {
	var price float64
	var err error
	if !strings.Contains(priceString, "null") && priceString != "" && priceString != "0" {
		price, err = strconv.ParseFloat(string(priceRegex.Find([]byte(priceString))), 64)
		if err != nil {
			panic(err)
		}
	} else {
		price = 0.0
	}
	return price
}

func getFilm(element *colly.HTMLElement) Film {
	yearString := element.Attr("data-film-release-year")
	year := getYear(yearString)
	film := Film{
		Slug:        element.Attr("data-target-link"),
		Url:         fmt.Sprintf("https://letterboxd.com/film%s", element.Attr("data-target-link")),
		Id:          element.Attr("data-film-id"),
		ServicesUrl: fmt.Sprintf("https://letterboxd.com/s/film-availability?filmId=%s&locale=USA", element.Attr("data-film-id")),
		Name:        element.Attr("data-film-name"),
		Year:        year,
	}
	return film
}

func getYear(yearString string) int {
	var year int
	var err error
	if yearString != "" {
		year, err = strconv.Atoi(yearString)
	}
	if err != nil {
		panic(err)
	}
	return year
}

func scrapeFilm(c *colly.Collector, url string) {
	c.OnHTML("section", func(e *colly.HTMLElement) {
		panic(e)
	})
	err := c.Visit(url)
	if err != nil {
		panic(err)
	}
}

func scrapeServices(ctx context.Context, element *colly.HTMLElement, servicesDataChan chan FilmPrices) {
	film := getFilm(element)
	filmPrices, ok := tryGetCachedFilm(ctx, film)
	if ok {
		servicesDataChan <- filmPrices
	} else {
		c := colly.NewCollector()
		c.OnResponse(func(e *colly.Response) {
			jsonParsed := parseJson(e)
			filmPrices := getServicesData(jsonParsed, film)
			cacheFilm(ctx, filmPrices)
			servicesDataChan <- filmPrices
		})
		err := c.Visit(film.ServicesUrl)
		if err != nil {
			panic(err)
		}
	}
}

func cacheFilm(ctx context.Context, filmPrices FilmPrices) {
	redisClient := cache.RedisClient()
	defer redisClient.Close()
	toCache := serialize(filmPrices)
	_, err := redisClient.Set(ctx, filmPrices.FilmDetails.Id, toCache, time.Duration(24)*time.Hour).Result()
	if err != redis.Nil && err != nil {
		panic(err)
	}
}

func tryGetCachedFilm(ctx context.Context, film Film) (FilmPrices, bool) {
	var filmPrices FilmPrices
	var ok bool
	redisClient := cache.RedisClient()
	defer redisClient.Close()
	cachedFilm, err := redisClient.Get(ctx, film.Id).Result()
	if err != redis.Nil && err != nil {
		panic(err)
	}
	if cachedFilm != "" {
		filmPrices = deserialize(cachedFilm)
		ok = true
	}
	return filmPrices, ok
}

func deserialize(cachedFilm string) FilmPrices {
	parsedJson, err := gabs.ParseJSON([]byte(cachedFilm))
	if err != nil {
		panic(err)
	}
	priceEntries := make([]PriceEntry, 0)
	prices := parsedJson.Path("PriceEntries")
	for _, price := range prices.Children() {
		priceEntries = append(priceEntries, PriceEntry{
			ServiceName: getUnquotedString(price.Path("ServiceName").String()),
			Format:      getUnquotedString(price.Path("Format").String()),
			Price:       getPrice(price.Path("Price").String()),
			PriceType:   getUnquotedString(price.Path("PriceType").String()),
			FilmName:    getUnquotedString(price.Path("FilmName").String()),
			FilmId:      getUnquotedString(price.Path("FilmId").String()),
			Url:         getUnquotedString(price.Path("Url").String()),
			Year:        getYear(price.Path("Year").String()),
		})
	}
	filmDetails := parsedJson.Path("FilmDetails")
	filmPrices := FilmPrices{
		FilmName: getUnquotedString(parsedJson.Path("FilmName").String()),
		FilmDetails: Film{
			Slug:        getUnquotedString(filmDetails.Path("Slug").String()),
			Url:         getUnquotedString(filmDetails.Path("Url").String()),
			Id:          getUnquotedString(filmDetails.Path("Id").String()),
			ServicesUrl: getUnquotedString(filmDetails.Path("ServicesUrl").String()),
			Name:        getUnquotedString(filmDetails.Path("Name").String()),
			Year:        getYear(filmDetails.Path("Year").String()),
		},
		PriceEntries: priceEntries,
	}
	return filmPrices
}

func getUnquotedString(myString string) string {
	priceType, err := strconv.Unquote(myString)
	if err != nil {
		panic(err)
	}
	return priceType
}

func serialize(filmPrices FilmPrices) []byte {
	res, err := json.Marshal(filmPrices)
	if err != nil {
		panic(err)
	}
	return res
}

func getDirectorUrl(name string) string {
	directorNameFmt := strings.Join(strings.Split(strings.ToLower(name), " "), "-")
	url := fmt.Sprintf("https://www.letterboxd.com/director/%s", directorNameFmt)
	return url
}
