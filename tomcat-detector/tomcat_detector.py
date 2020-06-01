from kubernetes import client, config, watch
import requests
import subprocess
import time
import re

httpd_pod_name = "httpd"
balancer_conf = "/usr/local/apache2/conf/balancer/proxy_balancer.conf"
httpd_conf= "/usr/local/apache2/bin/httpd"

def get_pod_IP(pod):
    if pod.status.pod_ip is not None:
        return str(pod.status.pod_ip)
    else:
        return pod.status.pod_ip

def pod_contains_tomcat(pod):
    for container in pod.spec.containers:
        if re.match("tomcat:.*", container.image) is not None:
            return True
    return False


def reload_httpd():
    print("Reloading httpd configuration.")
    httpd_reload_command = "kubectl,exec,httpd,--,"+httpd_conf+",-k,graceful"
    command_array = httpd_reload_command.split(",")
    subprocess.check_call(command_array, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT)
    print("")


def add_to_balancer(pod_ip, container_port):
    sed_expression = "s|\\(<Proxy \"balancer:.*>\\)|\\1\\n    BalancerMember \"http://" + pod_ip + ":"+ container_port +"\"|"
    add_balancer_member_command = [
        "kubectl", "exec", httpd_pod_name, "--", "sed", "-i", sed_expression, balancer_conf]
    subprocess.check_call(add_balancer_member_command)


def remove_from_balancer(pod_ip, container_port):
    sed_expression = "/    BalancerMember \"http:\/\/" + pod_ip + ":" + container_port + "\"/d"
    remove_balancer_member_command = [
        "kubectl", "exec", httpd_pod_name, "--", "sed", "-i", sed_expression, balancer_conf]
    subprocess.check_call(remove_balancer_member_command)



def get_tomcat_container_status(pod):
    for container_status in pod.status.container_statuses:
        if re.match("tomcat:.*", container_status.image):
            return container_status
        else:
            return None


def get_tomcat_container_specs(containers_specs, tomcat_status):
    for container_specs in containers_specs:
        if container_specs.image == tomcat_status.image:
            return container_specs
        else:
            return None


def container_is_active(pod, pod_status):
    liveness_conditions = [pod_status is not None,
                           pod_status.ready == True,
                           pod_status.state.running is not None,
                           pod.status.conditions[1].status == "True",  # (Pod)Ready ###
                           pod.status.conditions[2].status == "True",  # ContainersReady ###
                           pod.metadata.deletion_grace_period_seconds is None,
                           pod.metadata.deletion_timestamp is None]
    return all(liveness_conditions)


def tomcat_responds(pod_name, pod_ip, tomcat_port):
    i = 0
    while ++i <= 10:
        ret = requests.get("http://"+ pod_ip +":"+ tomcat_port)
        if ret.status_code == 200:
            return True
        time.sleep(1)
        if i == 10:
            return False

def main():
    config.load_incluster_config()
    tomcats = []
    v1 = client.CoreV1Api()

    w = watch.Watch()

    for event in w.stream(v1.list_pod_for_all_namespaces):
        #TODO lookout for httpd delete events
        #TODO handle no-existent httpd
        type = event['type']
        pod = event['object']
        pod_ip = get_pod_IP(pod)
        pod_name = pod.metadata.name
        if pod_contains_tomcat(pod):
            print("Namespace: %s Pod Name: %s Event: %s" %
                  (pod.metadata.namespace, pod.metadata.name, type))
            if pod_ip is not None:
                if pod.status.conditions is not None and \
                    pod.status.container_statuses is not None:

                    tomcat_status = get_tomcat_container_status(pod)
                    tomcat_specs = get_tomcat_container_specs(pod.spec.containers, tomcat_status)
                    container_active = container_is_active(pod, tomcat_status)
                    ## We assume that only one port is exposed on the tomcat container ###
                    container_port = str(tomcat_specs.ports[0].container_port)

                    if pod_ip in tomcats and not container_active:

                        print("Tomcat server in pod \""+pod_name +"\" went offline/crashed, removing it from proxy_balancer.conf")
                        remove_from_balancer(pod_ip, container_port)
                        tomcats.remove(pod_ip)
                        reload_httpd()

                    elif pod_ip not in tomcats and container_active:

                        if tomcat_responds(pod_name, pod_ip, container_port):
                            print("Tomcat server in pod \""+pod_name +"\" is online, adding it to proxy_balancer.conf")
                            add_to_balancer(pod_ip, container_port)
                            tomcats.append(pod_ip)
                            reload_httpd()

                        else:
                            print("Can't get a response from tomcat, check pod "+pod_name)

                    ### Checks if tomcat is in the process of being deleted to prevent unnecessary error messages ###
                    elif pod_ip not in tomcats and \
                        (pod.metadata.deletion_grace_period_seconds is not None or
                        pod.metadata.deletion_timestamp is not None):

                        pass

                    else:
                        print("Tomcat on pod \""+pod_name+"\" is offline.")
                else:
                    print("Pod status elements for pod \"" +pod_name+"\" are not initialized yet.")
            else:
                print("Pod \""+pod_name+"\" has no IP yet.")


if __name__ == "__main__":
    main()
