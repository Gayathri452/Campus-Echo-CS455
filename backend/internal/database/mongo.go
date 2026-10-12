package database

import (
	"context"
	"fmt"
	"regexp"
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
	otpCollection      *mongo.Collection
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
		otpCollection:      database.Collection("otp_challenges"),
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
	_, err = m.otpCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return fmt.Errorf("create OTP expiry index: %w", err)
	}
	_, err = m.feedbackCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "referenceCode", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{
			{Key: "referenceCode", Value: bson.D{{Key: "$type", Value: "string"}}},
		}),
	})
	if err != nil {
		return fmt.Errorf("create feedback reference index: %w", err)
	}
	_, err = m.appealCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "referenceCode", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{
			{Key: "referenceCode", Value: bson.D{{Key: "$type", Value: "string"}}},
		}),
	})
	if err != nil {
		return fmt.Errorf("create appeal reference index: %w", err)
	}
	return nil
}

func (m *Mongo) CreateStudent(ctx context.Context, email, passwordHash string) (auth.User, error) {
	now := time.Now().UTC()
	result, err := m.userCollection.InsertOne(ctx, bson.M{
		"email":        email,
		"passwordHash": passwordHash,
		"role":         auth.RoleStudent,
		"createdAt":    now,
	})
	if mongo.IsDuplicateKeyError(err) {
		user, _, findErr := m.FindUserByEmail(ctx, email)
		return user, findErr
	}
	if err != nil {
		return auth.User{}, fmt.Errorf("insert student: %w", err)
	}
	id, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return auth.User{}, fmt.Errorf("MongoDB returned unexpected student ID type %T", result.InsertedID)
	}
	return auth.User{ID: id.Hex(), Email: email, Role: auth.RoleStudent, CreatedAt: now}, nil
}

func (m *Mongo) SetStudentPassword(ctx context.Context, email, passwordHash string) error {
	result, err := m.userCollection.UpdateOne(ctx,
		bson.M{
			"email": email,
			"role":  auth.RoleStudent,
			"$or": []bson.M{
				{"passwordHash": bson.M{"$exists": false}},
				{"passwordHash": ""},
			},
		},
		bson.M{"$set": bson.M{"passwordHash": passwordHash}},
	)
	if err != nil {
		return fmt.Errorf("set student password: %w", err)
	}
	if result.MatchedCount == 0 {
		return auth.ErrInvalidCredentials
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

func (m *Mongo) DeleteModerator(ctx context.Context, email string) error {
	result, err := m.userCollection.DeleteOne(ctx, bson.M{"email": email, "role": auth.RoleModerator})
	if err != nil {
		return fmt.Errorf("delete moderator: %w", err)
	}
	if result.DeletedCount == 0 {
		return auth.ErrModeratorNotFound
	}
	return nil
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

func (m *Mongo) FindOTP(ctx context.Context, email string) (auth.OTPChallenge, error) {
	var result auth.OTPChallenge
	if err := m.otpCollection.FindOne(ctx, bson.M{"_id": email}).Decode(&result); err != nil {
		if err == mongo.ErrNoDocuments {
			return auth.OTPChallenge{}, auth.ErrInvalidOTP
		}
		return auth.OTPChallenge{}, fmt.Errorf("find OTP challenge: %w", err)
	}
	return result, nil
}

func (m *Mongo) InvalidateOTP(ctx context.Context, email string) error {
	_, err := m.otpCollection.DeleteOne(ctx, bson.M{"_id": email})
	if err != nil {
		return fmt.Errorf("invalidate OTP challenge: %w", err)
	}
	return nil
}

func (m *Mongo) CreateOTP(ctx context.Context, challenge auth.OTPChallenge) error {
	_, err := m.otpCollection.InsertOne(ctx, bson.M{
		"_id":         challenge.Email,
		"email":       challenge.Email,
		"otpHash":     challenge.OTPHash,
		"expiresAt":   challenge.ExpiresAt,
		"attempts":    challenge.Attempts,
		"requestedAt": challenge.RequestedAt,
		"resendCount": challenge.ResendCount,
	})
	if err != nil {
		return fmt.Errorf("insert OTP challenge: %w", err)
	}
	return nil
}

func (m *Mongo) IncrementOTPAttempts(ctx context.Context, email string) (int, error) {
	result := m.otpCollection.FindOneAndUpdate(ctx, bson.M{"_id": email}, bson.M{"$inc": bson.M{"attempts": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After))
	var challenge struct {
		Attempts int `bson:"attempts"`
	}
	if err := result.Decode(&challenge); err != nil {
		return 0, fmt.Errorf("increment OTP attempts: %w", err)
	}
	return challenge.Attempts, nil
}

func (m *Mongo) DeleteOTP(ctx context.Context, email string) error {
	_, err := m.otpCollection.DeleteOne(ctx, bson.M{"_id": email})
	if err != nil {
		return fmt.Errorf("delete OTP challenge: %w", err)
	}
	return nil
}

func (m *Mongo) Create(ctx context.Context, input feedback.CreateInput) (feedback.Feedback, error) {
	referenceCode, err := feedback.GenerateReferenceCode()
	if err != nil {
		return feedback.Feedback{}, err
	}
	now := time.Now().UTC()
	document := bson.M{
		"referenceCode": referenceCode,
		"courseId":      input.CourseID,
		"courseTitle":   input.CourseTitle,
		"category":      input.Category,
		"rating":        input.Rating,
		"content":       input.Content,
		"status":        feedback.StatusSubmitted,
		"createdAt":     now,
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
		ID:            id.Hex(),
		ReferenceCode: referenceCode,
		CourseID:      input.CourseID,
		CourseTitle:   input.CourseTitle,
		Category:      input.Category,
		Rating:        input.Rating,
		Content:       input.Content,
		Status:        feedback.StatusSubmitted,
		CreatedAt:     now,
	}, nil
}

func (m *Mongo) Explore(ctx context.Context, filters feedback.ExploreFilters) ([]feedback.Feedback, error) {
	query := bson.M{"status": feedback.StatusPublished}
	if filters.Category != "" {
		query["category"] = filters.Category
	}
	if filters.CourseID != "" {
		query["courseId"] = filters.CourseID
	}
	if filters.Rating >= feedback.MinRating && filters.Rating <= feedback.MaxRating {
		query["rating"] = filters.Rating
	}
	if filters.Search != "" {
		pattern := regexp.QuoteMeta(filters.Search)
		query["$or"] = []bson.M{
			{"courseId": primitive.Regex{Pattern: pattern, Options: "i"}},
			{"courseTitle": primitive.Regex{Pattern: pattern, Options: "i"}},
			{"category": primitive.Regex{Pattern: pattern, Options: "i"}},
			{"content": primitive.Regex{Pattern: pattern, Options: "i"}},
		}
	}
	options := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100)
	cursor, err := m.feedbackCollection.Find(ctx, query, options)
	if err != nil {
		return nil, fmt.Errorf("find published feedback: %w", err)
	}
	defer cursor.Close(ctx)
	var results []feedback.Feedback
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode published feedback: %w", err)
	}
	return results, nil
}

func (m *Mongo) CourseRatings(ctx context.Context, courseID string) ([]feedback.CourseRating, error) {
	match := bson.M{"status": feedback.StatusPublished}
	if courseID != "" {
		match["courseId"] = courseID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":           bson.M{"courseId": "$courseId", "courseTitle": "$courseTitle"},
			"reviewCount":   bson.M{"$sum": 1},
			"averageRating": bson.M{"$avg": "$rating"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "reviewCount", Value: -1}, {Key: "_id.courseId", Value: 1}}}},
	}
	cursor, err := m.feedbackCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate course ratings: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		ID struct {
			CourseID    string `bson:"courseId"`
			CourseTitle string `bson:"courseTitle"`
		} `bson:"_id"`
		ReviewCount   int64   `bson:"reviewCount"`
		AverageRating float64 `bson:"averageRating"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode course ratings: %w", err)
	}
	results := make([]feedback.CourseRating, 0, len(rows))
	for _, row := range rows {
		results = append(results, feedback.CourseRating{
			CourseID: row.ID.CourseID, CourseTitle: row.ID.CourseTitle,
			ReviewCount: row.ReviewCount, AverageRating: row.AverageRating,
		})
	}
	return results, nil
}

func (m *Mongo) FindByReference(ctx context.Context, referenceCode string) (feedback.Feedback, error) {
	var result feedback.Feedback
	if err := m.feedbackCollection.FindOne(ctx, bson.M{"referenceCode": referenceCode}).Decode(&result); err != nil {
		if err == mongo.ErrNoDocuments {
			return feedback.Feedback{}, feedback.ErrReviewNotFound
		}
		return feedback.Feedback{}, fmt.Errorf("find feedback by reference: %w", err)
	}
	return result, nil
}

func (m *Mongo) CreateAppeal(ctx context.Context, referenceCode, reason string) (feedback.Appeal, error) {
	review, err := m.FindByReference(ctx, referenceCode)
	if err != nil {
		return feedback.Appeal{}, err
	}
	if review.Status != feedback.StatusFlagged && review.Status != feedback.StatusRejected {
		return feedback.Appeal{}, feedback.ErrAppealNotEligible
	}
	appeal := feedback.Appeal{ReferenceCode: referenceCode, Reason: reason, Status: "pending", CreatedAt: time.Now().UTC()}
	if _, err := m.appealCollection.InsertOne(ctx, appeal); err != nil {
		return feedback.Appeal{}, fmt.Errorf("create appeal: %w", err)
	}
	return appeal, nil
}

func (m *Mongo) Disconnect(ctx context.Context) error {
	if err := m.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("disconnect MongoDB client: %w", err)
	}
	return nil
}
