#!/bin/bash
kubectl apply -f httpd-configmap.yml
kubectl apply -f proxy-balancer-configmap.yml