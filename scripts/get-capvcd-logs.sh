#!/bin/bash -e

# user input
if [ "$#" -lt 1 ]; then
    echo "ERROR: path to management cluster kubeconfig is not provided. Usage is ./get-capvcd-logs.sh [path-to-management-cluster-kubeconfig]"
fi

MANAGEMENT_KUBECONFIG="$1"

function logger() {
    echo "-------- START LOG $2" >> "${LOGDIR}/$1"
    /bin/bash -c "$2" >> "${LOGDIR}/$1" 2>&1
    exitCode="$?"

    # add a newline in case the command does not add it
    last=$(tail -c 1 "${LOGDIR}/$1")
    if [ -n "$last" ]; then
        echo >> "${LOGDIR}/$1"
    fi

    echo "-------- END   LOG $2 CODE $exitCode" >> "${LOGDIR}/$1"
}

function getWorkloadClusterLogs() {
    kubectl get secret --kubeconfig=${2} ${1}-kubeconfig -o json | jq ".data.value" | tr -d '"' | base64 -d > ${1}-kubeconfig.conf
    KUBECONFIG=${1}-kubeconfig.conf

    mkdir -p ${LOGDIR}

    echo "Collecting Kubernetes details for cluster \"${1}\""
    logger kubernetes.txt 'kubectl version'
    logger kubernetes.txt 'kubectl get nodes -owide'
    logger kubernetes.txt 'kubectl get pods -A -owide'
    logger kubernetes.txt 'kubectl get events -A'
    logger kubernetes.txt 'kubectl describe nodes'
    logger kubernetes.txt 'kubectl describe pods -A'
    logger kubernetes.txt 'kubectl describe deployments -A'
    logger kubernetes.txt 'kubectl describe replicasets -A'
    logger kubernetes.txt 'kubectl describe services -A'
    logger kubernetes.txt 'kubectl describe configmaps -A'
    logger kubernetes.txt 'kubectl get secrets -A'
    logger kubernetes.txt 'kubectl describe pv -A'

    echo "Collecting Kubernetes logs for cluster \"${1}\""
    kubectl cluster-info dump -A --output-directory="${LOGDIR}"/k8s-cluster-info &>/dev/null
}

function getManagementClusterLogs() {
    KUBECONFIG=${MANAGEMENT_KUBECONFIG}

    mkdir -p ${LOGDIR}

    echo "Collecting Kubernetes details for management cluster"
    logger kubernetes.txt 'kubectl version'
    logger kubernetes.txt 'kubectl get nodes -owide'
    logger kubernetes.txt 'kubectl get pods -A -owide'
    logger kubernetes.txt 'kubectl get events -A'
    logger kubernetes.txt 'kubectl describe nodes'
    logger kubernetes.txt 'kubectl describe pods -A'
    logger kubernetes.txt 'kubectl describe deployments -A'
    logger kubernetes.txt 'kubectl describe replicasets -A'
    logger kubernetes.txt 'kubectl describe services -A'
    logger kubernetes.txt 'kubectl describe configmaps -A'
    logger kubernetes.txt 'kubectl get secrets -A'
    logger kubernetes.txt 'kubectl describe pv -A'

    echo "Collecting Kubernetes logs for management cluster"
    kubectl cluster-info dump -A --output-directory="${LOGDIR}"/k8s-cluster-info &>/dev/null
}

PARENTDIR="logs-capvcd-"$(date '+%F-%H-%M-%S')
LOGDIR="${PARENTDIR}/logs-management-"$(date '+%F-%H-%M-%S')

mkdir -p ${PARENTDIR}
getManagementClusterLogs

for CLUSTERNAME in $(kubectl get clusters -o=jsonpath={.items..metadata.name})
do
    LOGDIR="${PARENTDIR}/logs-"${CLUSTERNAME}"-"$(date '+%F-%H-%M-%S')
    getWorkloadClusterLogs ${CLUSTERNAME} ${MANAGEMENT_KUBECONFIG}
done

echo "Compressing logs and generating tarball at ${PARENTDIR}/${PARENTDIR}.tar.gz"
tar -czvf "${PARENTDIR}/${PARENTDIR}.tar.gz" "${PARENTDIR}" &> /dev/null

exit 0
