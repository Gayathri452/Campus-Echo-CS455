package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestTriageMongoLeastPrivilegeIntegration(t *testing.T) {
	adminURI := os.Getenv("TRIAGE_MONGODB_TEST_ADMIN_URI")
	triageURI := os.Getenv("TRIAGE_MONGODB_TEST_URI")
	databaseName := os.Getenv("TRIAGE_MONGODB_TEST_DATABASE")
	if adminURI == "" && triageURI == "" && databaseName == "" {
		t.Skip("set TRIAGE_MONGODB_TEST_ADMIN_URI, TRIAGE_MONGODB_TEST_URI, and TRIAGE_MONGODB_TEST_DATABASE to verify MongoDB permissions")
	}
	if adminURI == "" || triageURI == "" || databaseName == "" {
		t.Fatal("all TRIAGE_MONGODB_TEST_* settings must be provided together")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminClient, err := mongo.Connect(ctx, options.Client().ApplyURI(adminURI))
	if err != nil {
		t.Fatalf("connect MongoDB test admin: %v", err)
	}
	if err := adminClient.Ping(ctx, nil); err != nil {
		t.Fatalf("ping MongoDB test admin: %v", err)
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := adminClient.Disconnect(disconnectCtx); err != nil {
			t.Errorf("disconnect MongoDB test admin: %v", err)
		}
	}()

	feedbackID := primitive.NewObjectID()
	userID := primitive.NewObjectID()
	feedbackCollection := adminClient.Database(databaseName).Collection("feedback")
	usersCollection := adminClient.Database(databaseName).Collection("users")
	_, err = feedbackCollection.InsertOne(ctx, bson.M{
		"_id": feedbackID, "referenceCode": "TRIAGE-" + feedbackID.Hex(),
		"courseId": "TRIAGE-PERMISSION-TEST", "courseTitle": "Triage permission test",
		"category": "teaching", "rating": 3,
		"content": "The course material is useful and clearly structured for students.",
		"status":  feedback.StatusSubmitted, "createdAt": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert test feedback as admin: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := feedbackCollection.DeleteOne(cleanupCtx, bson.M{"_id": feedbackID}); err != nil {
			t.Errorf("delete test feedback: %v", err)
		}
		if _, err := usersCollection.DeleteOne(cleanupCtx, bson.M{"_id": userID}); err != nil {
			t.Errorf("delete test identity: %v", err)
		}
	}()
	if _, err := usersCollection.InsertOne(ctx, bson.M{
		"_id": userID, "email": "triage-permission-test@example.invalid",
		"passwordHash": "test-only",
	}); err != nil {
		t.Fatalf("insert test identity as admin: %v", err)
	}

	triageStore, err := ConnectTriage(ctx, triageURI, databaseName)
	if err != nil {
		t.Fatalf("connect restricted triage store: %v", err)
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := triageStore.Disconnect(disconnectCtx); err != nil {
			t.Errorf("disconnect restricted triage store: %v", err)
		}
	}()

	review, err := triageStore.FindByID(ctx, feedbackID.Hex())
	if err != nil {
		t.Fatalf("restricted triage account could not read feedback: %v", err)
	}
	if review.ID != feedbackID.Hex() {
		t.Fatalf("triage read feedback ID %q, want %q", review.ID, feedbackID.Hex())
	}
	if _, claimed, err := triageStore.ClaimForTriage(ctx, feedbackID.Hex(), time.Minute); err != nil {
		t.Fatalf("restricted triage account could not update feedback lease: %v", err)
	} else if !claimed {
		t.Fatal("restricted triage account did not claim submitted feedback")
	}

	var identity bson.M
	err = triageStore.store.client.Database(databaseName).
		Collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&identity)
	if err == nil {
		t.Fatal("restricted triage account read an identity document; expected MongoDB authorization denial")
	}
	var commandError mongo.CommandError
	if !errors.As(err, &commandError) || commandError.Code != 13 {
		t.Fatalf("identity read error = %v, want MongoDB unauthorized error (code 13)", err)
	}
}
