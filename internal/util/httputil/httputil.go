/*
Copyright 2024 Alexandre Mahdhaoui

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package httputil

import (
	"net/http"
)

// BasicAuth is a middleware that performs basic authentication.
func BasicAuth(
	next http.Handler,
	validator func(username, password string, r *http.Request) (bool, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { //nolint:varnamelen
		// Extract the username and password from the request
		// Authorization header. If no Authentication header is present
		// or the header value is invalid, then the 'ok' return value
		// will be false.
		username, password, ok := r.BasicAuth()
		if ok {
			if ok, err := validator(username, password, r); err != nil {
				// If an error occurred during validation, then return a
				// 500 error.
				http.Error(w, err.Error(), http.StatusInternalServerError) // TODO: wrap error
			} else if ok {
				// If the username and password are correct, then call
				// the next handler in the chain. Make sure to return
				// afterwards, so that none of the code below is run.
				next.ServeHTTP(w, r)
				return
			}
		}

		// If the Authentication header is not present, is invalid, or the
		// username or password is wrong, then set a WWW-Authenticate
		// header to inform the client that we expect them to use basic
		// authentication and send a 401 Unauthorized response.
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
	}
}
