package linkschecker

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	parser "github.com/Eslam-Nawara/linkschecker/pkg/tomlparser"
	"golang.org/x/net/html"
)

type mapChanels struct {
	visitLink chan string
	linkState chan bool
}

// Parse the toml file, extract an array of links and start checking the links concurrently
func CheckLinksInFile(configFile string) error {

	links, err := parser.LinksFromConfig(configFile)

	if err != nil {
		return err
	}

	mp := manageLinksMap()

	sync := make(chan bool)
	go mp.checkArrayOfLinks(links, "", sync)
	<-sync

	return nil
}

// Starts a go routin that manages reading from and writing to the list
// of checked links that is shared among all go routins.
func manageLinksMap() mapChanels {
	ch := mapChanels{}

	ch.visitLink = make(chan string)
	ch.linkState = make(chan bool)

	visitedLinks := make(map[string]bool)
	go func() {
		for {
			link := <-ch.visitLink
			isVisited := visitedLinks[link]
			if !isVisited {
				visitedLinks[link] = true
			}
			ch.linkState <- isVisited
		}
	}()
	return ch
}

// Checks the health of an array of links
func (mp mapChanels) checkArrayOfLinks(links []string, parent string, parentChan chan bool) {

	cnt := 0
	childChan := make(chan bool)

	for _, link := range links {
		mp.visitLink <- link
		if ok := <-mp.linkState; !ok {

			tempLink := fmt.Sprintf("%s/%s", getHostname(parent), strings.Trim(link, "/"))

			if validateLink(tempLink) {
				innerLinks := visitLinkAndExtractLinks(tempLink)
				cnt++
				go mp.checkArrayOfLinks(innerLinks, tempLink, childChan)
			} else {
				if getHostname(link) == getHostname(parent) || parent == "" {
					if validateLink(link) {
						innerLinks := visitLinkAndExtractLinks(link)
						cnt++
						go mp.checkArrayOfLinks(innerLinks, link, childChan)
					} else {
						fmt.Println(link)
					}
				} else if !validateLink(link) {
					fmt.Println(link)
				}
			}
		}
	}
	for i := 0; i < cnt; i++ {
		<-childChan
	}
	parentChan <- true
}

// Validate the link by sending a Head request or a Get request.
func validateLink(link string) bool {

	link = ensureScheme(link)
	var requestFun func(fn func(string) (*http.Response, error)) bool

	requestFun = func(fn func(string) (*http.Response, error)) bool {
		resp, err := fn(link)
		if err != nil {
			return false
		}

		defer resp.Body.Close()
		statusCode := resp.StatusCode
		return (statusCode >= 200 && statusCode < 400)
	}
	return requestFun(http.Head) || requestFun(http.Get)
}

// Extract links all links in a web page.
func visitLinkAndExtractLinks(link string) []string {

	link = ensureScheme(link)
	resp, err := http.Get(link)

	if err != nil {
		return nil
	}

	body := resp.Body
	defer body.Close()

	return extractLinksFromIOReader(body)
}

func ensureScheme(link string) string {

	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		link = fmt.Sprintf("https://%s", link)
	}
	return link
}

// Extract the links from the page body
func extractLinksFromIOReader(body io.ReadCloser) []string {

	var links []string
	z := html.NewTokenizer(body)

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return links
		case html.StartTagToken, html.EndTagToken:
			token := z.Token()
			if "a" == token.Data {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						links = append(links, attr.Val)
					}
				}
			}
		}
	}
}

// Extract Hostname from a url
func getHostname(link string) string {

	link = ensureScheme(link)
	url, err := url.Parse(link)

	if err != nil {
		return ""
	}

	return strings.Trim(url.Hostname(), "/")
}
