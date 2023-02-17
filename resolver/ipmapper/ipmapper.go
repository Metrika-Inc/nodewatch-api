package ipmapper

import (
	"context"
	"net/http"

	"eth2-crawler/models"
	"eth2-crawler/resolver"
)

const (
	url = "http://host.docker.internal:8000/geoip"
)

type client struct {
	httpClient *http.Client
}

type geoInformation struct {
	Inetnum string `json:"netblock"`
	Country string `json:"country"`
}

func New() resolver.Provider {
	return &client{
		httpClient: &http.Client{},
	}
}

func (c *client) GetGeoLocation(ctx context.Context, ipAddr string) (*models.GeoLocation, error) {
	// req, err := http.NewRequest("GET", url, nil)
	// if err != nil {
	// 	return nil, err
	// }

	// q := req.URL.Query()
	// q.Add("ipaddress", ipAddr)
	// req.URL.RawQuery = q.Encode()

	// res, err := c.httpClient.Do(req)
	// if err != nil {
	// 	return nil, err
	// }

	// // nolint
	// defer res.Body.Close()
	// resBody, err := io.ReadAll(res.Body)
	// if err != nil {
	// 	return nil, fmt.Errorf("unable to read response body")
	// }

	// if res.StatusCode != http.StatusOK {
	// 	return nil, fmt.Errorf("invalid status code returned. statusCode::%d response::%s", res.StatusCode, string(resBody))
	// }

	// result := &geoInformation{}
	// err = json.Unmarshal(resBody, result)
	// if err != nil {
	// 	return nil, fmt.Errorf("unable to unmarshal body. error::%w", err)
	// }

	// geoLoc := &models.GeoLocation{
	// 	Country: result.Country,
	// }

	// return geoLoc, nil
	return &models.GeoLocation{}, nil
}
