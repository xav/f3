module github.com/xav/f3/apiserver

go 1.12

require (
	github.com/apex/log v1.1.0
	github.com/go-chi/chi v4.0.2+incompatible
	github.com/go-chi/render v1.0.1
	github.com/go-chi/valve v0.0.0-20170920024740-9e45288364f4
	github.com/google/uuid v1.1.1
	github.com/nats-io/nats.go v1.7.2
	github.com/pkg/errors v0.8.1
	github.com/spf13/cobra v0.0.4
	github.com/stretchr/testify v1.3.0
	github.com/xav/f3/apiservice v0.0.0-20190602163333-e1e9f149c270
	github.com/xav/f3/f3nats v0.0.0-20190602163333-e1e9f149c270
	github.com/xav/f3/models v0.0.0-20190602163333-e1e9f149c270
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
)
