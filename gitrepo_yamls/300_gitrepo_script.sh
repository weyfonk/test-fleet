n=300; step=50; for start in $(seq 1 $step $n) ; do for i in $(seq $start $((start + step - 1))); do cat <<EOF
---
kind: GitRepo
apiVersion: fleet.cattle.io/v1alpha1
metadata:
  name: lots-$i
  namespace: fleet-default
spec:
  repo: https://github.com/manno/fleet-experiments
  branch: main
  pollingInterval: 6h
  paths:
    - scale
  targetNamespace: lots-$i
  targets:
    - clusterSelector: {}
EOF
done | k apply -f -; sleep 30; done
