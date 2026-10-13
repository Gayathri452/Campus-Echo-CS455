// Command seed-courses adds the IIT Kanpur CSE course list to the course
// catalog. Courses already on the list are skipped, so it is safe to run
// more than once. From the backend folder:
//
//	go run ./cmd/seed-courses
//
// It uses the same MONGODB_URI / MONGODB_DATABASE settings as the API.
package main

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/config"
	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
	"github.com/Kishore200524/CS455-Project/backend/internal/database"
)

//go:embed iitk_cse_courses.txt
var courseList string

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	store, err := database.Connect(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		log.Fatalf("connect to MongoDB: %v", err)
	}
	defer store.Disconnect(context.Background())

	rows, err := parseCourseList(courseList)
	if err != nil {
		log.Fatal(err)
	}
	added, skipped := 0, 0
	for _, input := range rows {
		_, err := store.CreateCourse(ctx, courses.Course{CourseID: input.CourseID, Title: input.Title, Instructor: input.Instructor})
		switch {
		case errors.Is(err, courses.ErrCourseExists):
			skipped++
		case err != nil:
			log.Fatalf("add %s: %v", input.CourseID, err)
		default:
			added++
		}
	}
	fmt.Printf("Course list ready: %d added, %d already present (%d total in the file).\n", added, skipped, len(rows))
}

// parseCourseList reads "COURSEID, Title[, Instructor]" lines, skipping
// blank lines and lines starting with #.
func parseCourseList(text string) ([]courses.CreateInput, error) {
	var rows []courses.CreateInput
	scanner := bufio.NewScanner(strings.NewReader(text))
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.SplitN(raw, ",", 3)
		input := courses.CreateInput{CourseID: parts[0]}
		if len(parts) > 1 {
			input.Title = parts[1]
		}
		if len(parts) > 2 {
			input.Instructor = parts[2]
		}
		input = courses.Normalize(input)
		if err := courses.Validate(input); err != nil {
			return nil, fmt.Errorf("course list line %d (%q): %w", line, raw, err)
		}
		rows = append(rows, input)
	}
	return rows, scanner.Err()
}
