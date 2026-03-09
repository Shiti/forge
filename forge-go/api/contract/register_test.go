package contract

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterHandlers_NoPanic(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	require.NotPanics(t, func() {
		RegisterHandlers(r, UnimplementedServer{})
	})
}
