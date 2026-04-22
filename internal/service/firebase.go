package service

import (
	"context"
	"fmt"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type FirebaseService struct {
	AuthClient      *auth.Client
	MessagingClient *messaging.Client
	App             *firebase.App
	ready           bool
}

func NewFirebaseService(configPath string) *FirebaseService {
	svc := &FirebaseService{}

	if configPath == "" {
		log.Println("WARN: FIREBASE_CONFIG_PATH not set, Firebase disabled")
		return svc
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("WARN: Firebase config file not found at %s, Firebase disabled", configPath)
		return svc
	}

	ctx := context.Background()

	opt := option.WithCredentialsFile(configPath)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		log.Printf("WARN: Failed to initialize Firebase app: %v", err)
		return svc
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		log.Printf("WARN: Failed to initialize Firebase Auth client: %v", err)
		return svc
	}

	msgClient, err := app.Messaging(ctx)
	if err != nil {
		log.Printf("WARN: Failed to initialize Firebase Messaging client: %v", err)
	}

	log.Println("Firebase initialized successfully")
	svc.AuthClient = authClient
	svc.MessagingClient = msgClient
	svc.App = app
	svc.ready = true
	return svc
}

// IsMessagingReady reports whether FCM is usable.
func (f *FirebaseService) IsMessagingReady() bool {
	return f.ready && f.MessagingClient != nil
}

// SendMulticast delivers the same notification to a list of device tokens.
// Returns the list of successfully delivered tokens and the list of tokens that
// must be removed from the DB (unregistered / invalid).
func (f *FirebaseService) SendMulticast(
	ctx context.Context,
	tokens []string,
	title string,
	body string,
	data map[string]string,
) (successCount int, failureCount int, invalidTokens []string, err error) {
	if !f.IsMessagingReady() {
		return 0, 0, nil, fmt.Errorf("firebase messaging not configured")
	}
	if len(tokens) == 0 {
		return 0, 0, nil, nil
	}

	// FCM caps multicast at 500 tokens per request.
	const chunkSize = 500
	for start := 0; start < len(tokens); start += chunkSize {
		end := start + chunkSize
		if end > len(tokens) {
			end = len(tokens)
		}
		chunk := tokens[start:end]

		msg := &messaging.MulticastMessage{
			Tokens: chunk,
			Notification: &messaging.Notification{
				Title: title,
				Body:  body,
			},
			Data: data,
			APNS: &messaging.APNSConfig{
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Alert: &messaging.ApsAlert{
							Title: title,
							Body:  body,
						},
						Sound: "default",
					},
				},
			},
			Android: &messaging.AndroidConfig{
				Priority: "high",
				Notification: &messaging.AndroidNotification{
					Sound: "default",
				},
			},
		}

		resp, sendErr := f.MessagingClient.SendEachForMulticast(ctx, msg)
		if sendErr != nil {
			return successCount, failureCount, invalidTokens, sendErr
		}

		successCount += resp.SuccessCount
		failureCount += resp.FailureCount

		for i, r := range resp.Responses {
			if r.Success {
				continue
			}
			if r.Error != nil && (messaging.IsUnregistered(r.Error) || messaging.IsInvalidArgument(r.Error) || messaging.IsSenderIDMismatch(r.Error)) {
				invalidTokens = append(invalidTokens, chunk[i])
			}
		}
	}
	return successCount, failureCount, invalidTokens, nil
}

func (f *FirebaseService) IsReady() bool {
	return f.ready
}

func (f *FirebaseService) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	if !f.ready {
		return nil, fmt.Errorf("firebase not configured")
	}
	return f.AuthClient.VerifyIDToken(ctx, idToken)
}

func (f *FirebaseService) GetUser(ctx context.Context, uid string) (*auth.UserRecord, error) {
	if !f.ready {
		return nil, fmt.Errorf("firebase not configured")
	}
	return f.AuthClient.GetUser(ctx, uid)
}

type UserCount struct {
	Total int `json:"total"`
}

func (f *FirebaseService) GetUserCount(ctx context.Context) (*UserCount, error) {
	if !f.ready {
		return &UserCount{Total: -1}, fmt.Errorf("firebase not configured")
	}
	count := 0
	iter := f.AuthClient.Users(ctx, "")
	for {
		_, err := iter.Next()
		if err != nil {
			break
		}
		count++
	}
	return &UserCount{Total: count}, nil
}
