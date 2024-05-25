package db_service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DbService[DocType interface{}] interface {
	CreateDocument(ctx context.Context, id string, document *DocType) error
	FindDocument(ctx context.Context, id string) (*DocType, error)
	FindDocuments(ctx context.Context, filter bson.D) ([]*DocType, error)
	UpdateDocument(ctx context.Context, id string, document *DocType) error
	DeleteDocument(ctx context.Context, id string) error
	Disconnect(ctx context.Context) error
}

var ErrNotFound = fmt.Errorf("document not found")
var ErrConflict = fmt.Errorf("conflict: document already exists")

type MongoServiceConfig struct {
	ServerHost string
	ServerPort int
	UserName   string
	Password   string
	DbName     string
	Collection string
	Timeout    time.Duration
}

type mongoSvc[DocType interface{}] struct {
	MongoServiceConfig
	client     atomic.Pointer[mongo.Client]
	clientLock sync.Mutex
}

func NewMongoService[DocType interface{}](config MongoServiceConfig) DbService[DocType] {
	enviro := func(name string, defaultValue string) string {
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		return defaultValue
	}

	svc := &mongoSvc[DocType]{}
	svc.MongoServiceConfig = config

	if svc.ServerHost == "" {
		svc.ServerHost = enviro("AMBULANCE_API_MONGODB_HOST", "localhost")
	}

	if svc.ServerPort == 0 {
		port := enviro("AMBULANCE_API_MONGODB_PORT", "27017")
		if port, err := strconv.Atoi(port); err == nil {
			svc.ServerPort = port
		} else {
			log.Printf("Invalid port value: %v", port)
			svc.ServerPort = 27017
		}
	}

	if svc.DbName == "" {
		svc.DbName = enviro("AMBULANCE_API_MONGODB_DATABASE", "xcok-ambulance-wl")
	}

	if svc.Collection == "" {
		svc.Collection = enviro("AMBULANCE_API_MONGODB_COLLECTION", "ambulance")
	}

	if svc.Timeout == 0 {
		seconds := enviro("AMBULANCE_API_MONGODB_TIMEOUT_SECONDS", "10")
		if seconds, err := strconv.Atoi(seconds); err == nil {
			svc.Timeout = time.Duration(seconds) * time.Second
		} else {
			log.Printf("Invalid timeout value: %v", seconds)
			svc.Timeout = 10 * time.Second
		}
	}

	log.Printf(
		"MongoDB config: //%v@%v:%v/%v/%v",
		svc.UserName,
		svc.ServerHost,
		svc.ServerPort,
		svc.DbName,
		svc.Collection,
	)
	return svc
}

func (this *mongoSvc[DocType]) connect(ctx context.Context) (*mongo.Client, error) {
	client := this.client.Load()
	if client != nil {
		return client, nil
	}

	this.clientLock.Lock()
	defer this.clientLock.Unlock()
	client = this.client.Load()
	if client != nil {
		return client, nil
	}

	ctx, contextCancel := context.WithTimeout(ctx, this.Timeout)
	defer contextCancel()

	uri := fmt.Sprintf("mongodb://%v:%v", this.ServerHost, this.ServerPort)
	log.Printf("Using URI: %v", uri)

	clientOptions := options.Client().ApplyURI(uri).SetConnectTimeout(10 * time.Second)
	if len(this.UserName) != 0 && len(this.Password) != 0 {
		clientOptions.SetAuth(options.Credential{
			Username: this.UserName,
			Password: this.Password,
		})
	}

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}
	this.client.Store(client)
	return client, nil
}

func (this *mongoSvc[DocType]) Disconnect(ctx context.Context) error {
	client := this.client.Load()
	if client != nil {
		this.clientLock.Lock()
		defer this.clientLock.Unlock()
		client = this.client.Load()
		defer this.client.Store(nil)
		if client != nil {
			return client.Disconnect(ctx)
		}
	}
	return nil
}

func (this *mongoSvc[DocType]) CreateDocument(ctx context.Context, id string, document *DocType) error {
	ctx, contextCancel := context.WithTimeout(ctx, this.Timeout)
	defer contextCancel()
	client, err := this.connect(ctx)
	if err != nil {
		return err
	}
	db := client.Database(this.DbName)
	collection := db.Collection(this.Collection)
	result := collection.FindOne(ctx, bson.D{{Key: "id", Value: id}})
	switch result.Err() {
	case nil:
		return ErrConflict
	case mongo.ErrNoDocuments:
	default:
		return result.Err()
	}

	_, err = collection.InsertOne(ctx, document)
	return err
}

func (this *mongoSvc[DocType]) FindDocument(ctx context.Context, id string) (*DocType, error) {
	ctx, contextCancel := context.WithTimeout(ctx, this.Timeout)
	defer contextCancel()
	client, err := this.connect(ctx)
	if err != nil {
		return nil, err
	}
	db := client.Database(this.DbName)
	collection := db.Collection(this.Collection)
	result := collection.FindOne(ctx, bson.D{{Key: "id", Value: id}})
	if err := result.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var document DocType
	if err := result.Decode(&document); err != nil {
		return nil, err
	}
	return &document, nil
}

func (this *mongoSvc[DocType]) UpdateDocument(ctx context.Context, id string, document *DocType) error {
	ctx, contextCancel := context.WithTimeout(ctx, this.Timeout)
	defer contextCancel()
	client, err := this.connect(ctx)
	if err != nil {
		return err
	}
	db := client.Database(this.DbName)
	collection := db.Collection(this.Collection)
	result := collection.FindOne(ctx, bson.D{{Key: "id", Value: id}})
	if err := result.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrNotFound
		}
		return err
	}
	_, err = collection.ReplaceOne(ctx, bson.D{{Key: "id", Value: id}}, document)
	return err
}

func (this *mongoSvc[DocType]) DeleteDocument(ctx context.Context, id string) error {
	ctx, contextCancel := context.WithTimeout(ctx, this.Timeout)
	defer contextCancel()
	client, err := this.connect(ctx)
	if err != nil {
		return err
	}
	db := client.Database(this.DbName)
	collection := db.Collection(this.Collection)
	result := collection.FindOne(ctx, bson.D{{Key: "id", Value: id}})
	if err := result.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrNotFound
		}
		return err
	}
	_, err = collection.DeleteOne(ctx, bson.D{{Key: "id", Value: id}})
	return err
}

func (this *mongoSvc[DocType]) FindDocuments(ctx context.Context, filter bson.D) ([]*DocType, error) {
	ctx, contextCancel := context.WithTimeout(ctx, this.Timeout)
	defer contextCancel()
	client, err := this.connect(ctx)
	if err != nil {
		return nil, err
	}
	db := client.Database(this.DbName)
	collection := db.Collection(this.Collection)
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var documents []*DocType
	for cursor.Next(ctx) {
		var document DocType
		if err := cursor.Decode(&document); err != nil {
			return nil, err
		}
		documents = append(documents, &document)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return documents, nil
}
