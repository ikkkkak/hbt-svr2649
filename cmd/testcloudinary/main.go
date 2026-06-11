package main

import (
	"fmt"
	"os"

	"apartments-clone-server/storage"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	storage.InitializeMediaCDN()
	b64 := "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////2wBDAf//////////////////////////////////////////////////////////////////////////////////////wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAb/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAAX/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oADAMBAAIRAxEAPwCdABmX/9k="
	res := storage.UploadBase64Image(b64, "test_probe")
	fmt.Printf("url=%q error=%q\n", res["url"], res["error"])
	if res["url"] == "" {
		os.Exit(1)
	}
}
