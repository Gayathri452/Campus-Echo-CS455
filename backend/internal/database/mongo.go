package database

import (
	"context"
	"fmt"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	client             *mongo.Client
	feedbackCollection *mongo.Collection
	userCollection     *mongo.Collection
	sessionCollection  *mongo.Collection
	appealCollection   *mongo.Collection
}

func Connect(ctx context.Context, uri, databaseName string) (*Mongo, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("create MongoDB client: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if disconnectErr := client.Disconnect(disconnectCtx); disconnectErr != nil {
			return nil, fmt.Errorf("ping MongoDB: %w (disconnect client: %v)", err, disconnectErr)
		}
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}

	database := client.Database(databaseName)
	store := &Mongo{
		client:             client,
		feedbackCollection: database.Collection("feedback"),
		userCollection:     database.Collection("users"),
		sessionCollection:  database.Collection("sessions"),
		appealCollection:   database.Collection("appeals"),
	}
	if err := store.ensureAuthIndexes(ctx); err != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if disconnectErr := client.Disconnect(disconnectCtx); disconnectErr != nil {
			return nil, fmt.Errorf("create authentication indexes: %w (disconnect client: %v)", err, disconnectErr)
		}
		return nil, fmt.Errorf("create authentication indexes: %w", err)
	}
	return store, nil
}

func (m *Mongo) ensureAuthIndexes(ctx context.Context) error {
	_, err := m.userCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create unique user email index: %w", err)
	}

	_, err = m.sessionCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return fmt.Errorf("create session expiry index: %w", err)
	}
	return nil
}

func (m *Mongo) CreateUser(ctx context.Context, email, passwordHash, role string) (auth.User, error) {
	now := time.Now().UTC()
	document := bson.M{
		"email":        email,
		"passwordHash": passwordHash,
		"role":         role,
		"createdAt":    now,
	}
	result, err := m.userCollection.InsertOne(ctx, document)
	if mongo.IsDuplicateKeyError(err) {
		return auth.User{}, auth.ErrEmailAlreadyExists
	}
	if err != nil {
		return auth.User{}, fmt.Errorf("insert user: %w", err)
	}

	id, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return auth.User{}, fmt.Errorf("MongoDB returned unexpected user ID type %T", result.InsertedID)
	}
	return auth.User{ID: id.Hex(), Email: email, Role: role, CreatedAt: now}, nil
}

func (m *Mongo) FindUserByEmail(ctx context.Context, email string) (auth.User, string, error) {
	var result struct {
		ID           primitive.ObjectID `bson:"_id"`
		Email        string             `bson:"email"`
		PasswordHash string             `bson:"passwordHash"`
		Role         string             `bson:"role"`
		CreatedAt    time.Time          `bson:"createdAt"`
	}
	if err := m.userCollection.FindOne(ctx, bson.M{"email": email}).Decode(&result); err != nil {
		if err == mongo.ErrNoDocuments {
			return auth.User{}, "", auth.ErrInvalidCredentials
		}
		return auth.User{}, "", fmt.Errorf("find user by email: %w", err)
	}
	return auth.User{
		ID:        result.ID.Hex(),
		Email:     result.Email,
		Role:      result.Role,
		CreatedAt: result.CreatedAt,
	}, result.PasswordHash, nil
}

func (m *Mongo) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("parse session user ID: %w", err)
	}
	_, err = m.sessionCollection.InsertOne(ctx, bson.M{
		"_id":       tokenHash,
		"userId":    id,
		"expiresAt": expiresAt,
	})
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (m *Mongo) FindUserBySession(ctx context.Context, tokenHash string, now time.Time) (auth.User, error) {
	var session struct {
		UserID primitive.ObjectID `bson:"userId"`
	}
	err := m.sessionCollection.FindOne(ctx, bson.M{
		"_id":       tokenHash,
		"expiresAt": bson.M{"$gt": now},
	}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return auth.User{}, auth.ErrInvalidSession
		}
		return auth.User{}, fmt.Errorf("find session: %w", err)
	}

	var userRecord struct {
		Email     string    `bson:"email"`
		Role      string    `bson:"role"`
		CreatedAt time.Time `bson:"createdAt"`
	}
	err = m.userCollection.FindOne(ctx, bson.M{"_id": session.UserID}).Decode(&userRecord)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return auth.User{}, auth.ErrInvalidSession
		}
		return auth.User{}, fmt.Errorf("find session user: %w", err)
	}
	return auth.User{
		ID:        session.UserID.Hex(),
		Email:     userRecord.Email,
		Role:      userRecord.Role,
		CreatedAt: userRecord.CreatedAt,
	}, nil
}

func (m *Mongo) DeleteSession(ctx context.Context, tokenHash string) error {
	result, err := m.sessionCollection.DeleteOne(ctx, bson.M{"_id": tokenHash})
	if err != nil {
		return fmt.Errorf("delete session document: %w", err)
	}
	if result.DeletedCount == 0 {
		return auth.ErrInvalidSession
	}
	return nil
}

func (m *Mongo) Create(ctx context.Context, input feedback.CreateInput) (feedback.Feedback, error) {
	now := time.Now().UTC()
	document := bson.M{
		"courseId":  input.CourseID,
		"content":   input.Content,
		"status":    "submitted",
		"createdAt": now,
	}

	result, err := m.feedbackCollection.InsertOne(ctx, document)
	if err != nil {
		return feedback.Feedback{}, fmt.Errorf("insert feedback: %w", err)
	}

	id, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return feedback.Feedback{}, fmt.Errorf("MongoDB returned unexpected feedback ID type %T", result.InsertedID)
	}

	return feedback.Feedback{
		ID:        id.Hex(),
		CourseID:  input.CourseID,
		Content:   input.Content,
		Status:    "submitted",
		CreatedAt: now,
	}, nil
}

func (m *Mongo) Disconnect(ctx context.Context) error {
	if err := m.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("disconnect MongoDB client: %w", err)
	}
	return nil
}
