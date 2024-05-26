// package ambulance_wl

// import (
// 	"net/http"

// 	"github.com/DanielCok17/xcok-webapi/internal/db_service"
// 	"github.com/gin-gonic/gin"
// 	"github.com/google/uuid"
// )

// type AmbulancesAPI interface {
// 	// internal registration of api routes
// 	addRoutes(routerGroup *gin.RouterGroup)

// 	// CreateAmbulance - Saves new ambulance definition
// 	CreateAmbulance(ctx *gin.Context)

// 	// DeleteAmbulance - Deletes specific ambulance
// 	DeleteAmbulance(ctx *gin.Context)
// }

// type implAmbulancesAPI struct {
// 	dbService db_service.DbService[Ambulance] // Add the dbService field
// }

// func newAmbulancesAPI(dbService db_service.DbService[Ambulance]) AmbulancesAPI {
// 	return &implAmbulancesAPI{
// 		dbService: dbService, // Initialize the dbService field
// 	}
// }

// func (this *implAmbulancesAPI) addRoutes(routerGroup *gin.RouterGroup) {
// 	routerGroup.Handle(http.MethodPost, "/ambulance", this.CreateAmbulance)
// 	routerGroup.Handle(http.MethodDelete, "/ambulance/:ambulanceId", this.DeleteAmbulance)
// }

// // CreateAmbulance - Saves new ambulance definition
// func (this *implAmbulancesAPI) CreateAmbulance(ctx *gin.Context) {
// 	value, exists := ctx.Get("db_service")
// 	if !exists {
// 		ctx.JSON(
// 			http.StatusInternalServerError,
// 			gin.H{
// 				"status":  "Internal Server Error",
// 				"message": "db not found",
// 				"error":   "db not found",
// 			})
// 		return
// 	}

// 	db, ok := value.(db_service.DbService[Ambulance])
// 	if !ok {
// 		ctx.JSON(
// 			http.StatusInternalServerError,
// 			gin.H{
// 				"status":  "Internal Server Error",
// 				"message": "db context is not of required type",
// 				"error":   "cannot cast db context to db_service.DbService",
// 			})
// 		return
// 	}

// 	ambulance := Ambulance{}
// 	err := ctx.BindJSON(&ambulance)
// 	if err != nil {
// 		ctx.JSON(
// 			http.StatusBadRequest,
// 			gin.H{
// 				"status":  "Bad Request",
// 				"message": "Invalid request body",
// 				"error":   err.Error(),
// 			})
// 		return
// 	}

// 	if ambulance.Id == "" {
// 		ambulance.Id = uuid.New().String()
// 	}

// 	err = db.CreateDocument(ctx, ambulance.Id, &ambulance)

// 	switch err {
// 	case nil:
// 		ctx.JSON(
// 			http.StatusCreated,
// 			ambulance,
// 		)
// 	case db_service.ErrConflict:
// 		ctx.JSON(
// 			http.StatusConflict,
// 			gin.H{
// 				"status":  "Conflict",
// 				"message": "Ambulance already exists",
// 				"error":   err.Error(),
// 			},
// 		)
// 	default:
// 		ctx.JSON(
// 			http.StatusBadGateway,
// 			gin.H{
// 				"status":  "Bad Gateway",
// 				"message": "Failed to create ambulance in database",
// 				"error":   err.Error(),
// 			},
// 		)
// 	}
// }

// // DeleteAmbulance - Deletes specific ambulance
// func (this *implAmbulancesAPI) DeleteAmbulance(ctx *gin.Context) {
// 	value, exists := ctx.Get("db_service")
// 	if !exists {
// 		ctx.JSON(
// 			http.StatusInternalServerError,
// 			gin.H{
// 				"status":  "Internal Server Error",
// 				"message": "db_service not found",
// 				"error":   "db_service not found",
// 			})
// 		return
// 	}

// 	db, ok := value.(db_service.DbService[Ambulance])
// 	if !ok {
// 		ctx.JSON(
// 			http.StatusInternalServerError,
// 			gin.H{
// 				"status":  "Internal Server Error",
// 				"message": "db_service context is not of type db_service.DbService",
// 				"error":   "cannot cast db_service context to db_service.DbService",
// 			})
// 		return
// 	}

// 	ambulanceId := ctx.Param("ambulanceId")
// 	err := db.DeleteDocument(ctx, ambulanceId)

// 	switch err {
// 	case nil:
// 		ctx.AbortWithStatus(http.StatusNoContent)
// 	case db_service.ErrNotFound:
// 		ctx.JSON(
// 			http.StatusNotFound,
// 			gin.H{
// 				"status":  "Not Found",
// 				"message": "Ambulance not found",
// 				"error":   err.Error(),
// 			},
// 		)
// 	default:
// 		ctx.JSON(
// 			http.StatusBadGateway,
// 			gin.H{
// 				"status":  "Bad Gateway",
// 				"message": "Failed to delete ambulance from database",
// 				"error":   err.Error(),
// 			})
// 	}
// }

package ambulance_wl

import (
	"slices"
	"time"
)

// reconcileWaitingList updates the estimated start times for the waiting list entries
func (this *Ambulance) reconcileWaitingList() {
	slices.SortFunc(this.WaitingList, func(left, right WaitingListEntry) int {
		if left.WaitingSince.Before(right.WaitingSince) {
			return -1
		} else if left.WaitingSince.After(right.WaitingSince) {
			return 1
		} else {
			return 0
		}
	})

	// Assume the first entry's EstimatedStart is correct (computed before previous entry was deleted)
	// but cannot be before the current time
	// For simplicity, ignore the concept of opening hours here

	if this.WaitingList[0].EstimatedStart.Before(this.WaitingList[0].WaitingSince) {
		this.WaitingList[0].EstimatedStart = this.WaitingList[0].WaitingSince
	}

	if this.WaitingList[0].EstimatedStart.Before(time.Now()) {
		this.WaitingList[0].EstimatedStart = time.Now()
	}

	nextEntryStart := this.WaitingList[0].EstimatedStart.Add(time.Duration(this.WaitingList[0].EstimatedDurationMinutes) * time.Minute)
	for _, entry := range this.WaitingList[1:] {
		if entry.EstimatedStart.Before(nextEntryStart) {
			entry.EstimatedStart = nextEntryStart
		}
		if entry.EstimatedStart.Before(entry.WaitingSince) {
			entry.EstimatedStart = entry.WaitingSince
		}

		nextEntryStart = entry.EstimatedStart.Add(time.Duration(entry.EstimatedDurationMinutes) * time.Minute)
	}
}
