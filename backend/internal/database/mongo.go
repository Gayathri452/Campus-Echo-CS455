package database

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

type feedbackRecord struct {
	ID            primitive.ObjectID `bson:"_id"`
	ReferenceCode string             `bson:"referenceCode"`
	CourseID      string             `bson:"courseId"`
	CourseTitle   string             `bson:"courseTitle"`
	Category      string             `bson:"category"`
	Rating        int                `bson:"rating"`
	Content       string             `bson:"content"`
	Status        string             `bson:"status"`
	FlagReason    string             `bson:"flagReason,omitempty"`
	CreatedAt     time.Time          `bson:"createdAt"`
	PublishedAt   *time.Time         `bson:"publishedAt,omitempty"`
}

func (r feedbackRecord) feedback() feedback.Feedback {
	return feedback.Feedback{
		ID:            r.ID.Hex(),
		ReferenceCode: r.ReferenceCode,
		CourseID:      r.CourseID,
		CourseTitle:   r.CourseTitle,
		Category:      r.Category,
		Rating:        r.Rating,
		Content:       r.Content,
		Status:        r.Status,
		FlagReason:    r.FlagReason,
		CreatedAt:     r.CreatedAt,
		PublishedAt:   r.PublishedAt,
	}
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
	_, err = m.feedbackCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "status", Value: 1},
			{Key: "triageLeaseUntil", Value: 1},
			{Key: "createdAt", Value: 1},
		},
	})
	if err != nil {
		return fmt.Errorf("create submitted feedback recovery index: %w", err)
	}
	_, err = m.feedbackCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "courseId", Value: 1}, {Key: "createdAt", Value: -1}},
	})
	if err != nil {
		return fmt.Errorf("create course feedback lookup index: %w", err)
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
	var result feedbackRecord
	if err := m.feedbackCollection.FindOne(ctx, bson.M{"referenceCode": referenceCode}).Decode(&result); err != nil {
		if err == mongo.ErrNoDocuments {
			return feedback.Feedback{}, feedback.ErrReviewNotFound
		}
		return feedback.Feedback{}, fmt.Errorf("find feedback by reference: %w", err)
	}
	return result.feedback(), nil
}

func (m *Mongo) FindByID(ctx context.Context, id string) (feedback.Feedback, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return feedback.Feedback{}, fmt.Errorf("parse review ID: %w", err)
	}
	var result feedbackRecord
	if err := m.feedbackCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&result); err != nil {
		if err == mongo.ErrNoDocuments {
			return feedback.Feedback{}, feedback.ErrReviewNotFound
		}
		return feedback.Feedback{}, fmt.Errorf("find feedback by ID: %w", err)
	}
	return result.feedback(), nil
}

func (m *Mongo) ClaimForTriage(ctx context.Context, id string, lease time.Duration) (feedback.Feedback, bool, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return feedback.Feedback{}, false, fmt.Errorf("parse review ID for triage claim: %w", err)
	}
	now := time.Now().UTC()
	var record feedbackRecord
	err = m.feedbackCollection.FindOneAndUpdate(
		ctx,
		bson.M{
			"_id":    objectID,
			"status": feedback.StatusSubmitted,
			"$or": []bson.M{
				{"triageLeaseUntil": bson.M{"$exists": false}},
				{"triageLeaseUntil": bson.M{"$lte": now}},
			},
		},
		bson.M{"$set": bson.M{"triageLeaseUntil": now.Add(lease)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return feedback.Feedback{}, false, nil
		}
		return feedback.Feedback{}, false, fmt.Errorf("claim review for triage: %w", err)
	}
	return record.feedback(), true, nil
}

func (m *Mongo) ReleaseTriage(ctx context.Context, id string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("parse review ID for triage lease release: %w", err)
	}
	_, err = m.feedbackCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID, "status": feedback.StatusSubmitted},
		bson.M{"$unset": bson.M{"triageLeaseUntil": ""}},
	)
	if err != nil {
		return fmt.Errorf("release review triage lease: %w", err)
	}
	return nil
}

func (m *Mongo) UpdateStatus(ctx context.Context, id, status, reason string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("parse review ID: %w", err)
	}
	if status != feedback.StatusPublished && status != feedback.StatusFlagged {
		return fmt.Errorf("unsupported triage status %q", status)
	}
	set := bson.M{"status": status}
	unset := bson.M{"triageLeaseUntil": ""}
	if reason == "" {
		unset["flagReason"] = ""
	} else {
		set["flagReason"] = reason
	}
	if status == feedback.StatusPublished {
		set["publishedAt"] = time.Now().UTC()
	} else {
		unset["publishedAt"] = ""
	}
	update := bson.M{"$set": set, "$unset": unset}
	result, err := m.feedbackCollection.UpdateOne(ctx, bson.M{"_id": objectID, "status": feedback.StatusSubmitted}, update)
	if err != nil {
		return fmt.Errorf("update review status: %w", err)
	}
	if result.MatchedCount == 0 {
		return feedback.ErrReviewNotEligible
	}
	return nil
}

func (m *Mongo) FlagReviews(ctx context.Context, ids []string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("flag reason must not be empty")
	}
	objectIDs := make([]primitive.ObjectID, 0, len(ids))
	seen := make(map[primitive.ObjectID]struct{}, len(ids))
	for _, id := range ids {
		objectID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return fmt.Errorf("parse review ID for batch flag: %w", err)
		}
		if _, exists := seen[objectID]; exists {
			continue
		}
		seen[objectID] = struct{}{}
		objectIDs = append(objectIDs, objectID)
	}
	if len(objectIDs) == 0 {
		return errors.New("at least one review ID is required to flag a batch")
	}
	_, err := m.feedbackCollection.UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": objectIDs}, "status": feedback.StatusSubmitted},
		bson.M{
			"$set":   bson.M{"status": feedback.StatusFlagged, "flagReason": reason},
			"$unset": bson.M{"triageLeaseUntil": "", "publishedAt": ""},
		},
	)
	if err != nil {
		return fmt.Errorf("flag brigading review batch: %w", err)
	}
	return nil
}

func (m *Mongo) ListSubmitted(ctx context.Context, limit int) ([]feedback.Feedback, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now().UTC()
	cursor, err := m.feedbackCollection.Find(
		ctx,
		bson.M{
			"status": feedback.StatusSubmitted,
			"$or": []bson.M{
				{"triageLeaseUntil": bson.M{"$exists": false}},
				{"triageLeaseUntil": bson.M{"$lte": now}},
			},
		},
		options.Find().
			SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, fmt.Errorf("find submitted reviews for triage recovery: %w", err)
	}
	defer cursor.Close(ctx)
	var records []feedbackRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("decode submitted reviews for triage recovery: %w", err)
	}
	reviews := make([]feedback.Feedback, 0, len(records))
	for _, record := range records {
		reviews = append(reviews, record.feedback())
	}
	return reviews, nil
}

func (m *Mongo) ListRecentByCourse(ctx context.Context, courseID string, limit int) ([]feedback.Feedback, error) {
	if limit <= 0 {
		limit = 25
	}
	cursor, err := m.feedbackCollection.Find(ctx, bson.M{"courseId": courseID}, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list recent feedback for course: %w", err)
	}
	defer cursor.Close(ctx)
	var docs []feedbackRecord
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode recent feedback for course: %w", err)
	}
	result := make([]feedback.Feedback, 0, len(docs))
	for i := range docs {
		result = append(result, docs[i].feedback())
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
