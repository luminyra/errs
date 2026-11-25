// Command basic attaches slog attributes to errors across layers with errs and
// reads them back with typed keys.
package main

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/luminyra/errs"
)

// ErrNotFound is the sentinel error of the data layer.
var ErrNotFound = errors.New("not found")

// region: keys
// UserID is a typed attribute key: it accepts only an int and reads back an
// int.
const UserID errs.Key[int] = "user_id"

// endregion

// region: layers
// The domain layer attaches an error code, the user ID and a message that is
// safe to show to users.
func findUser(id int) error {
	return errs.Wrap(ErrNotFound, "find user",
		errs.Code.Attr("USER_NOT_FOUND"),
		errs.Public.Attr("We could not find that user."),
		UserID.Attr(id),
	)
}

// The API layer wraps the error again and adds the route. Any slog.Attr can be
// attached.
func handleGetUser(id int) error {
	return errs.Wrap(findUser(id), "get user", slog.String("route", "GET /users/{id}"))
}

// endregion

func main() {
	err := handleGetUser(42)

	// region: inspect
	fmt.Println(err)                         // the whole message chain
	fmt.Println(errors.Is(err, ErrNotFound)) // the sentinel still matches

	code, _ := errs.Code.From(err) // the innermost value in the chain
	public, _ := errs.Public.From(err)
	id, _ := UserID.From(err) // an int, no type assertion
	fmt.Println("code:  ", code)
	fmt.Println("public:", public)
	fmt.Println("userID:", id)

	if e, ok := errs.From(err); ok {
		for _, a := range e.Attrs() { // attributes merged across the chain
			fmt.Println("attr:  ", a)
		}
	}
	// endregion
}
