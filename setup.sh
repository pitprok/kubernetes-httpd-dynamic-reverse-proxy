#!/bin/bash
pushd proxy-balancer-automation/ || exit
docker build -f Dockerfile.multi -t proxy_balancer_automation:v1 .
popd || exit
kubectl apply -f service-account.yml
kubectl apply -f pod-modifier-role.yml
kubectl apply -f pod-modifier-binding.yml
kubectl apply -f proxy-balancer-automation-configmap.yml
sleep 5
kubectl apply -f proxy-balancer-automation.yml
