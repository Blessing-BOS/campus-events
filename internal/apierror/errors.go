package apierror

import "fmt"

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string                    { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func New(status int, code, message string) *Error { return &Error{status, code, message} }

var (
	Invalid      = New(400, "INVALID_ARGUMENT", "请求参数不正确")
	Unauthorized = New(401, "UNAUTHORIZED", "请重新登录")
	Forbidden    = New(403, "FORBIDDEN", "你没有执行此操作的权限")
	NotFound     = New(404, "NOT_FOUND", "记录不存在")
)
