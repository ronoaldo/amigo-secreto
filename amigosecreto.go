package amigosecreto

import "context"

func Health(ctx context.Context) (msg string, err error) {
	return "OK", nil
}
