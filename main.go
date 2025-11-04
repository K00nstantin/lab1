package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type PersonRequest struct {
	Name    *string `json:"name"`
	Age     *int32  `json:"age,omitempty"`
	Address *string `json:"address,omitempty"`
	Work    *string `json:"work,omitempty"`
}

type PersonResponse struct {
	ID      int32   `json:"id"`
	Name    string  `json:"name,omitempty"`
	Age     *int32  `json:"age,omitempty"`
	Address *string `json:"address,omitempty"`
	Work    *string `json:"work,omitempty"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type ValidationErrorResponse struct {
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors"`
}

type application struct {
	db *sql.DB
}

func (app *application) initDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/persons?sslmode=disable"
	}
	var err error
	app.db, err = sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err = app.db.Ping(); err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	createTable :=
		`CREATE TABLE IF NOT EXISTS persons (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		age INT,
		address TEXT,
		work TEXT
		);`

	_, err = app.db.Exec(createTable)
	if err != nil {
		return nil, fmt.Errorf("failed to create table %w", err)
	}
	return app.db, nil

}

func main() {
	app := &application{}
	db, err := app.initDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
		return
	}
	defer db.Close()

	r := mux.NewRouter()

	r.HandleFunc("/api/v1/persons", app.listPersons).Methods("GET")
	r.HandleFunc("/api/v1/persons", app.createPerson).Methods("POST")
	r.HandleFunc("/api/v1/persons/{id}", app.getPerson).Methods("GET")
	r.HandleFunc("/api/v1/persons/{id}", app.updatePerson).Methods("PATCH")
	r.HandleFunc("/api/v1/persons/{id}", app.deletePerson).Methods("DELETE")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func sendError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Message: message})
}

func sendValidationError(w http.ResponseWriter, message string, errors map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(ValidationErrorResponse{
		Message: message,
		Errors:  errors,
	})
}

func (app *application) listPersons(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query("SELECT id, name, age, address, work  FROM persons")
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Database query error")
		return
	}

	defer rows.Close()

	persons := []PersonResponse{}

	for rows.Next() {
		person := PersonResponse{}
		var age sql.NullInt64
		var address sql.NullString
		var work sql.NullString

		err = rows.Scan(&person.ID, &person.Name, &age, &address, &work)
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Rows scanning error")
			return
		}
		if age.Valid {
			ageval := int32(age.Int64)
			person.Age = &ageval
		}
		if address.Valid {
			addr := address.String
			person.Address = &addr
		}
		if work.Valid {
			wstr := work.String
			person.Work = &wstr
		}
		persons = append(persons, person)
	}
	if err = rows.Err(); err != nil {
		sendError(w, http.StatusInternalServerError, "Data iteration error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(persons)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "json encoding error")
		return
	}

}

func (app *application) createPerson(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req PersonRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		sendError(w, http.StatusBadRequest, "json decoding error")
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		sendValidationError(w, "name validation error", map[string]string{"name": "name is required"})
		return
	}
	var id int32
	err = app.db.QueryRow(
		"INSERT INTO persons (name, age, address, work) VALUES ($1, $2, $3, $4) RETURNING id",
		req.Name, req.Age, req.Address, req.Work,
	).Scan(&id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Query error")
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/persons/%d", id))
	w.WriteHeader(http.StatusCreated)
}

func (app *application) getPerson(w http.ResponseWriter, r *http.Request) {
	var person PersonResponse
	var age sql.NullInt32
	var address, work sql.NullString
	vars := mux.Vars(r)
	idstr := vars["id"]
	id, err := strconv.Atoi(idstr)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid ID format")
		return
	}
	err = app.db.QueryRow(
		"SELECT id, name, age, address, work FROM persons WHERE id = $1",
		id,
	).Scan(&person.ID, &person.Name, &age, &address, &work)
	if err == sql.ErrNoRows {
		sendError(w, http.StatusNotFound, "Person not found")
		return
	} else if err != nil {
		sendError(w, http.StatusInternalServerError, "Scanning error")
		return
	}
	if age.Valid {
		person.Age = &age.Int32
	}
	if address.Valid {
		addr := address.String
		person.Address = &addr
	}
	if work.Valid {
		wstr := work.String
		person.Work = &wstr
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(person)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Encoding error")
		return
	}
}

func (app *application) updatePerson(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idstr := vars["id"]
	id, err := strconv.Atoi(idstr)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid id format")
		return
	}

	var req struct {
		Name    *string `json:"name,omitempty"`
		Age     *int32  `json:"age,omitempty"`
		Address *string `json:"address,omitempty"`
		Work    *string `json:"work,omitempty"`
	}

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		sendValidationError(w, "Invalid json", map[string]string{"body": "invalid json format"})
		return
	}

	var exists bool
	err = app.db.QueryRow("SELECT EXISTS(SELECT 1 FROM persons WHERE id = $1)", id).Scan(&exists)
	if err != nil || !exists {
		sendError(w, http.StatusNotFound, "Person not found")
		return
	}

	var sname, saddress, swork sql.NullString
	var sage sql.NullInt32

	err = app.db.QueryRow("SELECT name, age, address, work FROM persons WHERE id = $1", id).Scan(&sname, &sage, &saddress, &swork)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Scanning error")
		return
	}

	if req.Name != nil {
		sname.String = *req.Name
		sname.Valid = true
	}

	if req.Age != nil {
		sage.Int32 = *req.Age
		sage.Valid = true
	}
	if req.Address != nil {
		saddress.String = *req.Address
		saddress.Valid = true
	}
	if req.Work != nil {
		swork.String = *req.Work
		swork.Valid = true
	}
	var fname, fage, faddress, fwork interface{} = nil, nil, nil, nil

	if sname.Valid {
		fname = sname.String
	}

	if sage.Valid {
		fage = sage.Int32
	}

	if saddress.Valid {
		faddress = saddress.String
	}

	if swork.Valid {
		fwork = swork.String
	}

	_, err = app.db.Exec("UPDATE persons SET name = $1, age = $2, address = $3, work = $4 WHERE id = $5",
		fname, fage, faddress, fwork, id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to update person")
		return
	}
	app.getPerson(w, r)
}

func (app *application) deletePerson(w http.ResponseWriter, r *http.Request) {
	if app.db == nil {
		sendError(w, http.StatusInternalServerError, "Database not initialized")
		return
	}
	vars := mux.Vars(r)
	idstr := vars["id"]
	id, err := strconv.Atoi(idstr)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid ID format")
		return
	}

	res, err := app.db.Exec("DELETE FROM persons WHERE id = $1", id)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Database error")
		return
	}

	rowaff, err := res.RowsAffected()
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if rowaff == 0 {
		sendError(w, http.StatusNotFound, "Person not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
