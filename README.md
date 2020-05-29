# kubernetes-httpd-dynamic-reverse-proxy

## Work in progress

### Before running setup.sh, you may need to uncomment lines based on the configuration of your system

Run setup.sh to set it up

and then run
`kubectl logs -f tomcat-detector`
and in a separate window run
`kubectl apply -f tomcat-deployment.yaml`
