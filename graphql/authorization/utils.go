package authorization

import (
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func ParseMaxAge(CacheControlHeaderStr string) (int64, error) {
	splittedHeaderStr := strings.Split(CacheControlHeaderStr, ",")
	for _, str := range splittedHeaderStr {
		strTrimSpace := strings.TrimSpace(str)
		if strings.HasPrefix(strTrimSpace, "max-age") || strings.HasPrefix(strTrimSpace, "s-maxage") {
			maxAge, err := strconv.Atoi(strings.Split(str, "=")[1])
			return int64(maxAge), err
		}
	}
	return 0, errors.Errorf("Couldn't Parse max-age")
}

func ParseExpires(ExpiresHeaderStr string) (int64, error) {
	expDate, err := time.Parse(time.RFC1123, ExpiresHeaderStr)
	if err != nil {
		return 0, err
	}
	currDate := time.Now().Round(time.Second)
	diff := expDate.Sub(currDate).Seconds()
	return int64(diff), nil
}
