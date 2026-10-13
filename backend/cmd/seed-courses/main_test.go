package main

import "testing"

func TestEmbeddedCourseListIsValid(t *testing.T) {
	rows, err := parseCourseList(courseList)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 54 {
		t.Fatalf("got %d courses, want 54", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.CourseID] {
			t.Fatalf("duplicate course %s", row.CourseID)
		}
		seen[row.CourseID] = true
	}
	if rows[0].CourseID != "CS201A" || rows[len(rows)-1].CourseID != "SE367A" {
		t.Fatalf("first/last = %s/%s", rows[0].CourseID, rows[len(rows)-1].CourseID)
	}
}

func TestParseCourseListRejectsBadLines(t *testing.T) {
	if _, err := parseCourseList("CS455A,"); err == nil {
		t.Fatal("missing title was accepted")
	}
	rows, err := parseCourseList("# comment\n\ncs455a, Software Engineering, Prof. Rao\n")
	if err != nil || len(rows) != 1 || rows[0].CourseID != "CS455A" || rows[0].Instructor != "Prof. Rao" {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
}
