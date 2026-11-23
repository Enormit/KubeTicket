module github.com/Enormit/kubeticket/services/flashsale-engine

go 1.27.4

require (
	github.com/Enormit/kubeticket/services/pkg v0.0.0
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/go-chi/chi/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/redis/go-redis/v9 v9.7.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
)

replace github.com/Enormit/kubeticket/services/pkg => ../pkg
