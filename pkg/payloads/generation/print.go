package generation

import (
	"fmt"
)

func PrintGeneratedPayload(payload *Payload) {
	fmt.Printf("Payload:\n")
	fmt.Println(payload.Value)
	fmt.Println("\nDetection Methods:")
	for _, dm := range payload.DetectionMethods {
		fmt.Println(dm.GetMethod())
	}
	fmt.Println("\nVars:")
	for _, v := range payload.Vars {
		fmt.Printf("%s: %s\n", v.Name, v.Value)
	}

}
