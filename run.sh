docker build -t video-transcoder:latest .

docker run --rm \
   --name video-proxy \
   --device /dev/dri:/dev/dri \
   --network=workspace_default \
   -p 8080:8080 \
   video-transcoder:latest
