package permission

var (
	whitelistMap = map[string]bool{
		"/version":            true,
		"/prometheus/metrics": true,
	}
)

func InWhiteList(url string) bool {
	if _, ok := whitelistMap[url]; ok {
		return true
	}
	return false
}
