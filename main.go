package lambda-extention-client

import (
	"context"
	"fmt"
)

func Run(ctx context.Context) error {
	fmt.Println("lambda-extention-client!")
	return nil
}
