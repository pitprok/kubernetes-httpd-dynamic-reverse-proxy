#!/bin/bash
pushd tomcat-detector/
docker build -t tomcat_detector:alpha .
popd
kubectl apply -f pod-reader-role.yaml
kubectl apply -f pod-reader-binding.yaml
kubectl apply -f httpd-config-map.yaml
kubectl apply -f httpd.yaml
kubectl apply -f tomcat-detector.yaml