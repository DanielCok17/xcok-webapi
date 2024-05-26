package ambulance_wl

import (
	"github.com/DanielCok17/xcok-webapi/internal/db_service"
	"github.com/gin-gonic/gin"
)

func AddRoutes(engine *gin.Engine, dbService db_service.DbService[Ambulance]) {
	group := engine.Group("/api")

	{
		api := newAmbulanceConditionsAPI()
		api.addRoutes(group)
	}

	{
		api := newAmbulanceWaitingListAPI()
		api.addRoutes(group)
	}

	// {
	// 	api := newAmbulancesAPI(dbService) // Pass the dbService instance
	// 	api.addRoutes(group)
	// }
}
