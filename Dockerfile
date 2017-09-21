FROM scratch
COPY kubeaudit /
ENTRYPOINT ["/kubeaudit"]
