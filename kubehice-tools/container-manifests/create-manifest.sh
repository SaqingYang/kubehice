docker manifest create --insecure $1 "$1"-amd64 "$1"-arm64
docker manifest annotate $1 "$1"-amd64 --os linux --arch amd64
docker manifest annotate $1 "$1"-arm64 --os linux --arch arm64
docker manifest push --insecure $1

