# Proxy-balancer Automation

The goal of this project is to automatically detect new back-end servers in a kubernetes cluster and dynamically add/remove them to/from the balancer members of httpd's mod_proxy_balancer.
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

Installation instructions

https://kubernetes.io/docs/tasks/tools/install-minikube/

- Warning: While using minikube, the following command has to be run before building the proxy-balancer-automation docker image

  `eval $(minikube docker-env)`

## Setup guide

1. Open the file proxy-balancer-automation-configmap.yml and set the following parameters

   - httpdPodName

   Name of the pod containing the httpd instance that will act as reverse-proxy

   - httpdContainerName

   The name of the container in the pod in which the instance of httpd exists

   - httpdBinary

   The location of the httpd binary in httpd's container

   - proxyBalancerConf

   The location of the configuration for mod_proxy_balancer in httpd's container

   - tomcatImage

   The name and version of the tomcat image used. This also works with regex, for example tomcat:.\* accepts all versions of the official tomcat distribution

   - tomcatLabels (optional filter)

   Here the user may set which labels the tomcat pod should have, in order to be included.

2. Run setup.sh which automatically builds the docker image, creates the necessary service account, role and role binding, and deploys the automation pod.

Alternatively, each .yml file can be modified separately to suit the needs of the user and applied individually. You can find details about the use of each component below.

## Testing the application

- Using the provided demo

  1. Go into the test/ directory

  2. Run test-setup.sh

  3. `kubectl apply -f httpd.yml`

  4. `kubectl apply -f tomcat-deployment.yml`

- With your own custom httpd & tomcat

  1. Deploy an instance of the httpd server

     - Make sure that mod_proxy_balancer is enabled in your httpd conf file and you have created the necessary balancer configuration. An example can be found in test/proxy-balancer-configmap.yml under "proxy_balancer.conf"

       https://httpd.apache.org/docs/2.4/mod/mod_proxy_balancer.html

  2. Deploy an instance of the tomcat server

The web server will automatically appear in your mod_proxy_balancer configuration.
You can also check the logs of the automation pod.

## Components analysis

- proxy balancer automation

The program creates a new API watch and receives updates for every pod-related action.

It checks if the pod is related to our reverse-proxy or if it fulfils the criteria for a balancer member server and acts accordingly.

- service-account.yml

Creates a new service account which will subsequently obtain the additional permissions required to communicate with the Kubernetes API and use kubectl to automatically edit and refresh httpd's configuration.

- pod-modifier-role.yml

Creates a role with permissions to query the Kubernetes API for information about pods and use kubectl exec.

- pod-modifier-binding.yml

Binds the pod-modifier role to the new service account, which the proxy-balancer-automation will be using.

- proxy-balancer-automation-configmap.yml

Contains all the user-specific information required for the program to work.
