package common

import "context"

type Processor interface {
	Name() string
	Process(ctx context.Context, input []byte) ([]byte, error)
}

// // API types
// export interface ApiResponse<T> {
//   data: T;
//   success: boolean;
//   error?: string;
// }

// export interface ApiError {
//   message: string;
//   code: string;
//   status: number;
// }

// API types
type ApiResponse[T any] struct {
	Data      T       `json:"data"`
	Success   bool    `json:"success"`
	Message   string  `json:"message,omitempty"`
	ErrorCode *string `json:"errorCode,omitempty"`
	Error     *string `json:"error,omitempty"`
}
