# Proxy-balancer Automation

The focus of this program is to automatically detect new back-end servers in a kubernetes cluster and dynamically add/remove them to/from the balancer members of httpd's mod_proxy_balancer.
For the time being the only back-end server supported is tomcat.

## Prerequisites

### Docker

Installation instructions
`https://docs.docker.com/engine/install/`

### kubectl (min. version 1.11.0)

Installation instructions
`https://kubernetes.io/docs/tasks/tools/install-kubectl/`

### Kubernetes

This application is meant to be used in a kubernetes cluster

To use/test locally, minikube can be used (min. version v1.4.0)
Warning: While using minikube, the following command has to be run before building the proxy-balancer-automation docker image
`eval $(minikube docker-env)`
Installation instructions
`https://kubernetes.io/docs/tasks/tools/install-minikube/`

## Setup guide

Open the file proxy-balancer-automation-configmap.yml and set the following parameters

httpdPodName
Name of the pod containing the httpd instance that will act as reverse-proxy

httpdContainerName
The name of the container in the pod in which the instance of httpd exists

httpdBinary
The location of the httpd binary in httpd's container

proxyBalancerConf
The location of the configuration for mod_proxy_balancer in httpd's container

tomcatImage
The name and version of the tomcat image used. This also works with regex, for example tomcat:.\* accepts all versions of the official tomcat distribution

tomcatLabels (optional)
Here the user may set any labels the pod containing tomcat should have in order to be included.

After configuring the selection criteria and providing the locations of the necessary files, running setup.sh automatically builds the docker image, creates the necessary service account, role and role binding, and deploys the pod.

Alternatively, each .yml file can be modified separately to suit the needs of the user and applied individually. You can find details about the use of each component below.

## Testing the application

After creating an httpd and a web server pod that match the criteria set in the installation guide, the web server will automatically appear in your mod_proxy_balancer configuration file.

## Components analysis

proxy balancer automation

The program creates a new API watch and receives updates for every pod-related action.

It checks if the pod is related to our reverse-proxy or if it fulfils the criteria for a balancer member server and acts accordingly.

service-account.yml

Creates a new service account which will subsequently obtain the additional permissions required to communicate with the Kubernetes API and use kubectl to automatically edit and refresh httpd's configuration.

pod-modifier-role.yml

Creates a role with permissions to query the Kubernetes API for information about pods and use kubectl exec.

pod-modifier-binding.yml

Binds the pod-modifier role to the new service account, which the proxy-balancer-automation will be using.

proxy-balancer-automation-configmap.yml

Contains all the user-specific information required for the program to work.
