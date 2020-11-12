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

func ScrapeDirector(name string, streamingServices []string) ([]FilmPrices, string) {
	url := getDirectorUrl(name)
	filmElements := scrapeFilms(url)
	films, overallPrice := scrapePrices(filmElements, streamingServices)
	return films, overallPrice
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
	FilmName     string
	PriceEntries []PriceEntry
	FilmDetails Film
	Streaming []PriceEntry
	CheapestRental PriceEntry
}

func scrapePrices(elements []*colly.HTMLElement, streamingServices []string) ([]FilmPrices, string) {
	var films []FilmPrices
	var servicesDataChan = make(chan FilmPrices, len(elements))

	// Use the worker pool pattern, see https://gobyexample.com/worker-pools
	for _, element := range elements {
		// scrapeFilm(c, film.Url)
		go scrapeServices(element, servicesDataChan)
	}
	for i := 0; i < len(elements); i++ {
		film := <-servicesDataChan
		film = film.getBestPrices(streamingServices)
		films = append(films, film)
	}
	fmt.Println("Before sorting")
	for _, film := range films {
		fmt.Println(film.FilmDetails.Name, film.FilmDetails.Year)
	}
	sortFilmsByYear(films)
	fmt.Println("After sorting")
	for _, film := range films {
		fmt.Println(film.FilmDetails.Name, film.FilmDetails.Year)
	}
	overallPrice := calculateOverallPrice(films)
	return films, overallPrice
}

func calculateOverallPrice(films []FilmPrices) string {
	var price float64
	for _, film := range films {
		if len(film.Streaming) == 0 {
			price += film.CheapestRental.Price
		}
	}
	return fmt.Sprintf("$%.2f", price)
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
		price := getPrice(child)
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
	yearString := element.Attr("data-film-release-year")
	var year int
	var err error
	if yearString != "" {
		year, err = strconv.Atoi(yearString)
	}
	if err != nil {
		fmt.Println(err)
	}
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

func scrapeFilm(c *colly.Collector, url string) {
	c.OnHTML("section", func(e *colly.HTMLElement) {
		fmt.Println(e)
	})
	err := c.Visit(url)
	if err != nil {
		panic(err)
	}
}

func scrapeServices(element *colly.HTMLElement, servicesDataChan chan FilmPrices) {
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
