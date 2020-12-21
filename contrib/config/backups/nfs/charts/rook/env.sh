## global
export NFS_STRATEGY="rook"

## values for rook
export NFS_SERVER="rook-nfs"
export NFS_PATH="share1"
## storage to use by NFS server
export NFS_DISK_SIZE="32Gi"
## storage to use from NFS server
export NFS_CLIAM_SIZE="32Gi"
export NFS_CLAIM_NAME="rook-nfs-pv-claim"

## values for dgraph (dynamic = will supply PVC claim to Dgraph)
export VOL_TYPE="volume"
