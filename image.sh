# TAG
# TAG
# TAG
# TAG
PROJECT_NAME=go-do
DOCKER_REGISTRY="docker-registry.phaseiilabs.com"
TAG="1.0.0"
# TAG
# TAG
# TAG
# TAG

_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

set -e
set -x

DOCKER_IMAGE_URL="${DOCKER_REGISTRY}/${PROJECT_NAME}"

docker build --platform linux/amd64 --progress=plain -t "${DOCKER_IMAGE_URL}:${TAG}" -f Dockerfile .

docker push "${DOCKER_IMAGE_URL}:${TAG}"

set +x
echo "Visit:";
echo -e "\n\t- https://$DOCKER_REGISTRY/v2/_catalog\n\t- https://$DOCKER_REGISTRY/v2/$PROJECT_NAME/tags/list";
set -x