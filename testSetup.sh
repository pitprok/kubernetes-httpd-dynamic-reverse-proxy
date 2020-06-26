#!/bin/bash
### Uncomment the following command to build
### the tomcat-detector image in minikube
# eval $(minikube docker-env)
kubectl apply -f service-account.yml
kubectl apply -f pod-modifier-role.yml
kubectl apply -f pod-modifier-binding.yml
kubectl apply -f httpd-configmap.yml
kubectl apply -f proxy-balancer-configmap.yml
kubectl apply -f tomcat-detector-configmap.yml
sleep 5
kubectl apply -f tomcat-detector.yml
