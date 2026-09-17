package main

import (
	"log"
	"net/http"
	"os"

	"restaurant-reservations/internal/reservation"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	store, err := reservation.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer store.Close()

	service := reservation.NewService(store, reservation.DefaultClock)
	log.Printf("restaurant reservations API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, reservation.NewHandler(service)))
}
