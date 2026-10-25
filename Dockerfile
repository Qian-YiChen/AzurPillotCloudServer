# 多阶段构建：静态编译 + 极简运行镜像。
FROM golang:1.26-alpine AS build
WORKDIR /src
# 先拷贝依赖清单，利用层缓存。
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X ap-cloud-server/internal/version.Version=${VERSION}" \
    -o /out/ap-cloud-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ap-cloud-server /ap-cloud-server
# 静态发布目录（未配置 CF 凭据时本地生成用）。
WORKDIR /app
VOLUME ["/app/public"]
ENV PUBLISH_DIR=/app/public
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ap-cloud-server"]
