package utils

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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

func DumpHtmlTable(tplPath string, data interface{}, output string) {

	tmpl, err := template.ParseFiles(tplPath)
	if err != nil {
		log.Fatal(err)
	}

	fobj, err := os.OpenFile(output, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panic(err.Error())
	}
	defer fobj.Close()

	err = tmpl.Execute(fobj, data)
	if err != nil {
		log.Fatal(err)
	}
}
