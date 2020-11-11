package scraper

import (
	"fmt"
	"github.com/Jeffail/gabs/v2"
	"github.com/gocolly/colly/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func ScrapeDirector(name string, streamingServices []string) ([]FilmEntry, string) {
	url := getDirectorUrl(name)
	filmElements := scrapeFilms(url)
	films, overallPrice := scrapePrices(filmElements, streamingServices)
	return films, overallPrice
}

type FilmEntry struct {
	FilmName string
	Streaming []PriceEntry
	CheapestRental PriceEntry
}

func scrapePrices(elements []*colly.HTMLElement, streamingServices []string) ([]FilmEntry, string) {
	var prices []FilmPriceEntry
	var servicesDataChan = make(chan FilmPriceEntry, len(elements))

	// Use the worker pool pattern, see https://gobyexample.com/worker-pools
	for _, element := range elements {
		// scrapeFilm(c, film.Url)
		go scrapeServices(element, servicesDataChan)
	}
	for i := 0; i < len(elements); i++ {
		prices = append(prices, <-servicesDataChan)
	}
	films := getBestPrices(prices, streamingServices)
	overallPrice := calculateOverallPrice(films)
	return films, overallPrice
}

func calculateOverallPrice(films []FilmEntry) string {
	var price float64
	for _, film := range films {
		if len(film.Streaming) == 0 {
			price += film.CheapestRental.Price
		}
	}
	return fmt.Sprintf("$%.2f", price)
}

func getBestPrices(prices []FilmPriceEntry, streamingServices []string) []FilmEntry {
	films := make([]FilmEntry, 0)
	for _, price := range prices {
		streamingSites := filterPrices(price.PriceEntries, filterStreamingServices(streamingServices))
		rentalPrices := filterPrices(price.PriceEntries, func(price PriceEntry) bool {
			return price.PriceType == "rent"
		})
		sortPrices(rentalPrices)
		if len(streamingSites) > 0 || len(rentalPrices) > 0 {
			cheapestRentalSite := rentalPrices[0]
			films = append(films, FilmEntry{FilmName: price.FilmName, Streaming: streamingSites, CheapestRental: cheapestRentalSite})
		} else {
			films = append(films, FilmEntry{FilmName: price.FilmName})
		}
	}
	return films
}

func sortPrices(prices []PriceEntry) {
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Price < prices[j].Price
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

type Film struct {
	Slug        string
	Url         string
	Id          string
	ServicesUrl string
	Name        string
}

type PriceEntry struct {
	ServiceName string
	Format      string
	Price       float64
	PriceType   string
	FilmName    string
	FilmId      string
	Url         string
}

type FilmPriceEntry struct {
	FilmName string
	PriceEntries []PriceEntry
}

func getServicesData(jsonParsed *gabs.Container, film Film) FilmPriceEntry {
	rent := jsonParsed.Path("best.rent")
	streaming := jsonParsed.Path("best.stream")
	prices := make([]PriceEntry, 0)
	for _, price := range getPrices(film, "streaming", streaming) {
		prices = append(prices, price)
	}
	for _, price := range getPrices(film, "rent", rent) {
		prices = append(prices, price)
	}
	filmPriceEntry := FilmPriceEntry{FilmName: film.Name, PriceEntries: prices}
	return filmPriceEntry
}

func getPrices(film Film, priceType string, prices *gabs.Container) []PriceEntry {
	priceData := make([]PriceEntry, 0)
	for _, child := range prices.Children() {
		price := getPrice(child)
		priceData = append(priceData, PriceEntry{
			ServiceName: strings.Trim(child.Path("name").String(), "\""),
			Format:      strings.Trim(child.Path("format").String(), "\""),
			Price:       price,
			PriceType:   priceType,
			FilmName:    film.Name,
			FilmId:      film.Id,
			Url:         film.ServicesUrl,
		})
	}
	return priceData
}

var priceRegex = regexp.MustCompile(`\d+\.\d+`)

func getPrice(child *gabs.Container) float64 {
	priceString := child.Path("price").String()
	var price float64
	var err error
	if !strings.Contains(priceString, "null") {
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
	film := Film{
		Slug:        element.Attr("data-target-link"),
		Url:         fmt.Sprintf("https://letterboxd.com/film%s", element.Attr("data-target-link")),
		Id:          element.Attr("data-film-id"),
		ServicesUrl: fmt.Sprintf("https://letterboxd.com/s/film-availability?filmId=%s&locale=USA", element.Attr("data-film-id")),
		Name:        element.Attr("data-film-name"),
	}
	return film
}

func scrapeFilm(c *colly.Collector, url string) {
	c.OnHTML("section", func(e *colly.HTMLElement) {
		fmt.Println(e)
	})
	err := c.Visit(url)
	if err != nil {
		panic(err)
	}
}

func scrapeServices(element *colly.HTMLElement, servicesDataChan chan FilmPriceEntry) {
	film := getFilm(element)
	c := colly.NewCollector()
	c.OnResponse(func(e *colly.Response) {
		jsonParsed := parseJson(e)
		filmPrices := getServicesData(jsonParsed, film)
		servicesDataChan <- filmPrices
	})
	err := c.Visit(film.ServicesUrl)
	if err != nil {
		panic(err)
	}
}

func getDirectorUrl(name string) string {
	directorNameFmt := strings.Join(strings.Split(strings.ToLower(name), " "), "-")
	url := fmt.Sprintf("https://www.letterboxd.com/director/%s", directorNameFmt)
	return url
}
