/*
 * Copyright (C) 2018 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package endpoints

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/mysteriumnetwork/node/session"
	"github.com/mysteriumnetwork/node/tequilapi/utils"
)

// serviceSessionsList defines session list representable as json
// swagger:model ServiceSessionListDTO
type serviceSessionsList struct {
	Sessions []serviceSession `json:"sessions"`
}

// serviceSession represents the session object
// swagger:model serviceSessionDTO
type serviceSession struct {
	// example: 4cfb0324-daf6-4ad8-448b-e61fe0a1f918
	ID string `json:"id"`

	// example: 0x0000000000000000000000000000000000000001
	ConsumerID string `json:"consumerId"`
}

type serviceSessionsEndpoint struct{}

// NewServiceSessionsEndpoint creates and returns sessions endpoint
func NewServiceSessionsEndpoint() *serviceSessionsEndpoint {
	return &serviceSessionsEndpoint{}
}

// swagger:operation GET /service/:id/sessions ServiceSessions listSessions
// ---
// summary: Returns current sessions
// description: Returns list of sessions in currently running service
// responses:
//   200:
//     description: List of sessions
//     schema:
//       "$ref": "#/definitions/ServiceSessionListDTO"
//   500:
//     description: Internal server error
//     schema:
//       "$ref": "#/definitions/ErrorMessageDTO"
func (endpoint *serviceSessionsEndpoint) List(resp http.ResponseWriter, request *http.Request, params httprouter.Params) {
	sessions := make([]session.Session, 0)

	sessionsSerializable := serviceSessionsList{
		Sessions: mapServiceSessions(sessions, serviceSessionToDto),
	}
	utils.WriteAsJSON(sessionsSerializable, resp)
}

func serviceSessionToDto(se session.Session) serviceSession {
	return serviceSession{
		ID:         string(se.ID),
		ConsumerID: se.ConsumerID.Address,
	}
}

func mapServiceSessions(sessions []session.Session, f func(session.Session) serviceSession) []serviceSession {
	dtoArray := make([]serviceSession, len(sessions))
	for i, se := range sessions {
		dtoArray[i] = f(se)
	}
	return dtoArray
}
