package database

import (
	"errors"
	"testing"

	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
)

func TestCourseCatalogIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)

	for _, course := range []courses.Course{
		{CourseID: "MTH101", Title: "Mathematics I", Instructor: "Prof. Iyer"},
		{CourseID: "CS455", Title: "Software Engineering", Instructor: "Prof. Rao"},
	} {
		if _, err := store.CreateCourse(ctx, course); err != nil {
			t.Fatalf("create %s: %v", course.CourseID, err)
		}
	}
	if _, err := store.CreateCourse(ctx, courses.Course{CourseID: "CS455", Title: "Duplicate"}); !errors.Is(err, courses.ErrCourseExists) {
		t.Fatalf("duplicate create = %v, want ErrCourseExists", err)
	}

	all, err := store.ListCourses(ctx, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].CourseID != "CS455" || all[1].CourseID != "MTH101" {
		t.Fatalf("list = %+v, want CS455 then MTH101", all)
	}
	if all[0].CreatedAt.IsZero() {
		t.Fatal("createdAt was not set")
	}

	found, err := store.ListCourses(ctx, "software", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].CourseID != "CS455" {
		t.Fatalf("search by title = %+v", found)
	}
	byInstructor, err := store.ListCourses(ctx, "iyer", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(byInstructor) != 1 || byInstructor[0].CourseID != "MTH101" {
		t.Fatalf("search by instructor = %+v", byInstructor)
	}

	if err := store.DeleteCourse(ctx, "CS455"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCourse(ctx, "CS455"); !errors.Is(err, courses.ErrCourseNotFound) {
		t.Fatalf("second delete = %v, want ErrCourseNotFound", err)
	}
}
