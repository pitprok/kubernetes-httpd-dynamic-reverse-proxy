#!/bin/bash
### Uncomment the following command to build
### the tomcat-detector image in minikube
#eval $(minikube docker-env)
pushd tomcat-detector/ || exit
docker build -f Dockerfile.multi -t tomcat_detector:alpha .
popd || exit
kubectl apply -f service-account.yaml
kubectl apply -f pod-modifier-role.yaml
kubectl apply -f pod-modifier-binding.yaml
kubectl apply -f httpd-configmap.yaml
kubectl apply -f proxy-balancer-configmap.yaml
kubectl apply -f tomcat-detector-configmap.yaml
kubectl apply -f httpd.yaml
kubectl apply -f tomcat-detector.yaml
