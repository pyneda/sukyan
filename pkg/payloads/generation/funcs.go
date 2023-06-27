package generation

import (
	"github.com/pyneda/sukyan/lib"
	"strconv"
)

func getTemplateFuncs() map[string]interface{} {
	return map[string]interface{}{
		"base64encode":                  lib.Base64Encode,
		"base64decode":                  lib.Base64Decode,
		"generateInteractionUrl":        generateInteractionUrl,
		"genRandInt":                    lib.GenerateRandInt,
		"generateRandomString":          lib.GenerateRandomString,
		"generateRandomLowercaseString": lib.GenerateRandomLowercaseString,
	}
}

func generateInteractionUrl() string {
	// Just return a static string as a POC
	return "http://example.com/" + strconv.Itoa(lib.GenerateRandInt(1, 1000)) + "/"
}
