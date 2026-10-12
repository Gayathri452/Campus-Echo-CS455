// Command seed-appeals inserts fake appeal tickets (and the flagged reviews they
// point at) into the local development database, so the moderator pages have
// something to claim before the student-appeal feature exists.
//
// Run from the backend directory with MongoDB running:
//
//	go run ./cmd/seed-appeals
//
// It is safe to run repeatedly: it first removes everything it inserted before
// (documents marked "seed": true) and never touches real data. Development use
// only.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type seedAppeal struct {
	courseID  string
	year      int
	professor string
	feedback  string
	reason    string
	appeal    string
	age       time.Duration // how long ago the appeal was raised
	status    string
	lockedBy  string        // only for locked tickets
	lockAge   time.Duration // how long ago the lock was taken
}

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatalf("connect to MongoDB: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Printf("disconnect from MongoDB: %v", err)
		}
	}()

	database := client.Database(cfg.MongoDatabase)
	feedbackCollection := database.Collection("feedback")
	appealCollection := database.Collection("appeals")

	for _, collection := range []*mongo.Collection{feedbackCollection, appealCollection} {
		if _, err := collection.DeleteMany(ctx, bson.M{"seed": true}); err != nil {
			log.Fatalf("remove earlier seed data from %s: %v", collection.Name(), err)
		}
	}

	now := time.Now().UTC()
	seeds := []seedAppeal{
		{
			courseID: "CS455", year: 2026, professor: "Prof. Rao",
			feedback: "The projects were too long and the grading felt arbitrary.",
			reason:   "Possibly abusive language", appeal: "This is harsh but fair criticism.",
			age: 5 * time.Hour, status: appeals.StatusUnderAppeal,
		},
		{
			courseID: "CS340", year: 2026, professor: "Prof. Iyer",
			feedback: "Lectures were disorganised and the slides were never updated.",
			reason:   "Flagged as abusive by triage", appeal: "I only described what happened.",
			age: 4 * time.Hour, status: appeals.StatusUnderAppeal,
		},
		{
			courseID: "MTH201", year: 2025, professor: "Prof. Sen",
			feedback: "Exams did not match what was taught in class.",
			reason:   "Possibly abusive language", appeal: "Please re-check; nothing here is abusive.",
			age: 3 * time.Hour, status: appeals.StatusUnderAppeal,
		},
		{
			courseID: "PHY101", year: 2025, professor: "Prof. Das",
			feedback: "Not a great course overall, the labs were frustrating.",
			reason:   "Flagged as abusive by triage", appeal: "Frustrating is not abusive.",
			age: 1 * time.Hour, status: appeals.StatusUnderAppeal,
		},
		{
			// Locked for 45 minutes: the lock has expired, so this must show up in
			// the queue again (tests FR-12 even before the sweep runs).
			courseID: "CS230", year: 2026, professor: "Prof. Nair",
			feedback: "Assignments had unclear requirements every single week.",
			reason:   "Possibly abusive language", appeal: "Unclear requirements are a fair complaint.",
			age: 2 * time.Hour, status: appeals.StatusLocked,
			lockedBy: "seed-other-moderator", lockAge: 45 * time.Minute,
		},
		{
			// Locked 5 minutes ago by someone else: must NOT appear in the queue.
			courseID: "EE210", year: 2026, professor: "Prof. Khan",
			feedback: "The instructor rushed through the hardest topics.",
			reason:   "Flagged as abusive by triage", appeal: "This is honest feedback.",
			age: 90 * time.Minute, status: appeals.StatusLocked,
			lockedBy: "seed-other-moderator", lockAge: 5 * time.Minute,
		},
	}

	for i, seed := range seeds {
		created := now.Add(-seed.age)

		review, err := feedbackCollection.InsertOne(ctx, bson.M{
			"courseId":  seed.courseID,
			"content":   seed.feedback,
			"status":    "under_appeal",
			"createdAt": created,
			"seed":      true,
		})
		if err != nil {
			log.Fatalf("insert seed review: %v", err)
		}
		reviewID, ok := review.InsertedID.(primitive.ObjectID)
		if !ok {
			log.Fatalf("MongoDB returned unexpected review ID type %T", review.InsertedID)
		}

		ticket := bson.M{
			"reviewId":      reviewID.Hex(),
			"referenceCode": fmt.Sprintf("CE-SEED-%02d", i+1),
			"courseId":      seed.courseID,
			"year":          seed.year,
			"professor":     seed.professor,
			"status":        seed.status,
			"feedbackText":  seed.feedback,
			"flagReason":    seed.reason,
			"appealComment": seed.appeal,
			"createdAt":     created,
			"seed":          true,
		}
		if seed.status == appeals.StatusLocked {
			ticket["lockedBy"] = seed.lockedBy
			ticket["lockedAt"] = now.Add(-seed.lockAge)
		}
		if _, err := appealCollection.InsertOne(ctx, ticket); err != nil {
			log.Fatalf("insert seed appeal: %v", err)
		}
	}

	fmt.Printf("Inserted %d seed appeals into %s.appeals\n", len(seeds), cfg.MongoDatabase)
}
