package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

func testDBURL() string {
	if v := os.Getenv("TEST_DB_URL"); v != "" {
		return v
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return "postgres://program:test@localhost:5432/persons?sslmode=disable"
	}
	return "postgres://program:test@localhost:5433/persons?sslmode=disable"
}

func stringPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32    { return &i }

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("postgres", testDBURL())
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping test database: %v", err)
	}
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS persons (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		age INT,
		address TEXT,
		work TEXT
	)`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	_, err = db.Exec("DELETE FROM persons")
	if err != nil {
		t.Fatalf("Failed to clean test table: %v", err)
	}

	return db
}

func setupTestRouterWithDB(t *testing.T) (*mux.Router, *application) {
	db := setupTestDB(t)
	app := &application{db: db}

	r := mux.NewRouter()
	r.HandleFunc("/api/v1/persons", app.listPersons).Methods("GET")
	r.HandleFunc("/api/v1/persons", app.createPerson).Methods("POST")
	r.HandleFunc("/api/v1/persons/{id}", app.getPerson).Methods("GET")
	r.HandleFunc("/api/v1/persons/{id}", app.updatePerson).Methods("PATCH")
	r.HandleFunc("/api/v1/persons/{id}", app.deletePerson).Methods("DELETE")

	return r, app
}

func createJSONBody(data interface{}) *bytes.Buffer {
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(data)
	return &buf
}

func TestCreateAndGetPerson(t *testing.T) {
	router, app := setupTestRouterWithDB(t)
	defer app.db.Close()

	person := PersonRequest{
		Name:    stringPtr("Test User"),
		Age:     int32Ptr(25),
		Address: stringPtr("Test Address"),
		Work:    stringPtr("Test Work"),
	}

	body := createJSONBody(person)
	req, _ := http.NewRequest("POST", "/api/v1/persons", body)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("Expected status 201, got %d. Response: %s", status, rr.Body.String())
	}

	location := rr.Header().Get("Location")
	var id string
	fmt.Sscanf(location, "/api/v1/persons/%s", &id)

	req, _ = http.NewRequest("GET", "/api/v1/persons/"+id, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Response: %s", status, rr.Body.String())
	}

	var personResp PersonResponse
	err := json.NewDecoder(rr.Body).Decode(&personResp)
	if err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if personResp.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", personResp.Name)
	}
}

func TestCreatePerson_Validation(t *testing.T) {
	router, app := setupTestRouterWithDB(t)
	defer app.db.Close()

	testCases := []struct {
		name         string
		person       PersonRequest
		expectedCode int
		description  string
	}{
		{
			name: "Empty name",
			person: PersonRequest{
				Name: stringPtr(""),
			},
			expectedCode: http.StatusBadRequest,
			description:  "Должен вернуть 400 при пустом имени",
		},
		{
			name: "Missing name",
			person: PersonRequest{
				Name: nil,
			},
			expectedCode: http.StatusBadRequest,
			description:  "Должен вернуть 400 при отсутствии имени",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := createJSONBody(tc.person)
			req, _ := http.NewRequest("POST", "/api/v1/persons", body)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tc.expectedCode {
				t.Errorf("%s: Expected status %d, got %d", tc.description, tc.expectedCode, status)
			}
		})
	}
}

func TestGetPerson_NotFound(t *testing.T) {
	router, app := setupTestRouterWithDB(t)
	defer app.db.Close()

	req, _ := http.NewRequest("GET", "/api/v1/persons/9999", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", status)
	}
}

func TestListPersons(t *testing.T) {
	router, app := setupTestRouterWithDB(t)
	defer app.db.Close()

	persons := []PersonRequest{
		{Name: stringPtr("User 1"), Age: int32Ptr(20)},
		{Name: stringPtr("User 2"), Age: int32Ptr(30)},
	}

	for _, person := range persons {
		body := createJSONBody(person)
		req, _ := http.NewRequest("POST", "/api/v1/persons", body)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
	}

	req, _ := http.NewRequest("GET", "/api/v1/persons", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status 200, got %d", status)
	}

	var personsResp []PersonResponse
	err := json.NewDecoder(rr.Body).Decode(&personsResp)
	if err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if len(personsResp) < 2 {
		t.Errorf("Expected at least 2 persons, got %d", len(personsResp))
	}
}

func TestDeletePerson(t *testing.T) {
	router, app := setupTestRouterWithDB(t)
	defer app.db.Close()

	person := PersonRequest{Name: stringPtr("To Delete")}
	body := createJSONBody(person)
	req, _ := http.NewRequest("POST", "/api/v1/persons", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	location := rr.Header().Get("Location")
	var id string
	fmt.Sscanf(location, "/api/v1/persons/%s", &id)

	req, _ = http.NewRequest("DELETE", "/api/v1/persons/"+id, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", status)
	}
}
