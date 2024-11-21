package utils

import (
	"encoding/json"
	"fmt"
	"os"
)

func DumpToFile(data []byte, path string) {
	fobj, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panic(err.Error())
	}
	defer fobj.Close()
	_, err = fobj.Write(data)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}

func DumpToJsonFile(data interface{}, path string) {
	output, err := json.Marshal(data)
	if err != nil {
		println("json dump failed:", err.Error())
	}
	DumpToFile(output, path)
}
