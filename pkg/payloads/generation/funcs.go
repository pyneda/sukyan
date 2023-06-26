package generation

import (
	"math/rand"
	"strconv"
	"time"
)

func generateInteractionUrl() string {
	// Just return a static string as a POC
	return "http://example.com/" + strconv.Itoa(genRandInt(1, 1000)) + "/"
}

// Custom function to generate a random integer
func genRandInt(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min+1) + min
}
