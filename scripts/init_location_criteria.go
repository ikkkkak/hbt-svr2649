package main

import (
	"apartments-clone-server/routes"
	"apartments-clone-server/storage"
	"fmt"
	"log"
)

func main() {
	// Initialize database
	storage.InitializeDB()

	// Initialize location criteria
	if err := routes.InitializeLocationCriteria(); err != nil {
		log.Fatalf("Error initializing location criteria: %v", err)
	}

	// Assign properties to criteria
	if err := routes.AssignPropertiesToCriteria(); err != nil {
		log.Fatalf("Error assigning properties to criteria: %v", err)
	}

	fmt.Println("Location criteria initialization completed successfully!")
}
