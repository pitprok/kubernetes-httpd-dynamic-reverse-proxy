from kubernetes import client, config, watch
import requests
import subprocess
import time


def pod_contains_tomcat(pod):
    if pod.spec.containers[0].image == "tomcat:9.0" and \
            pod.metadata.labels['category'] == "test-dynamic-reverse-proxy":
        return True
    return False


def reload_httpd():
    print("Restarting httpd.")
    subprocess.check_call(["kubectl", "exec", "httpd", "--container",
                           "httpd", "--", "/usr/local/apache2/bin/httpd", "-k", "graceful"])
    print("")


def add_to_balancer(pod_ip, tomcat_port):
    subprocess.check_call(["kubectl", "exec", "httpd", "--container",
                           "httpd", "--", "sed", "-i", "s|\\(<Proxy \"balancer:.*>\\)|\\1\\n    BalancerMember \"http://" +
                           str(pod_ip) + ":"+str(tomcat_port)+"\"|", "/usr/local/apache2/conf/balancer/proxy_balancer.conf"])


def remove_from_balancer(pod_ip, tomcat_port):
    subprocess.check_call(["kubectl", "exec", "httpd", "--container",
                           "httpd", "--", "sed", "-i", "/    BalancerMember \"http:\/\/"+str(pod_ip) + ":" +
                           str(tomcat_port)+"\"/d", "/usr/local/apache2/conf/balancer/proxy_balancer.conf"])


def main():
    config.load_incluster_config()
    tomcats = []
    v1 = client.CoreV1Api()

    w = watch.Watch()

    for event in w.stream(v1.list_pod_for_all_namespaces):
        # type = event['type']
        pod = event['object']
        pod_ip = pod.status.pod_ip
        pod_name = pod.metadata.name
        if pod_contains_tomcat(pod):
            # print("Namespace: %s Pod Name: %s Event: %s" % (pod.metadata.namespace, pod.metadata.name, event['type']))
            if pod.status.conditions is not None and pod.status.container_statuses is not None:
                if pod_ip is not None:
                    liveness_conditions = [pod.status.conditions[0].type == "Initialized",
                                           pod.status.conditions[1].status == "True",
                                           pod.status.conditions[2].status == "True",
                                           pod.status.container_statuses[0].image == "tomcat:9.0",
                                           pod.status.container_statuses[0].state.running is not None,
                                           pod.metadata.deletion_grace_period_seconds is None,
                                           pod.metadata.deletion_timestamp is None]
                    if pod_ip in tomcats and not all(liveness_conditions):
                        # TODO Find tomcat container and port in pod with multiple containers
                        tomcat_port = pod.spec.containers[0].ports[0].container_port
                        print("Tomcat in pod "+pod_name +
                              " went offline/crashed, removing it from proxy_balancer.conf")
                        remove_from_balancer(pod_ip, tomcat_port)
                        tomcats.remove(pod_ip)
                        reload_httpd()
                    elif pod_ip not in tomcats and all(liveness_conditions):
                        i = 0
                        # TODO Find tomcat container and port in pod with multiple containers
                        tomcat_port = pod.spec.containers[0].ports[0].container_port
                        while i < 10 and pod_ip not in tomcats:
                            ret = requests.get(
                                "http://"+str(pod_ip)+":"+str(tomcat_port))
                            if ret.status_code == 200:
                                print("Tomcat in pod "+pod_name +
                                      " is online, adding it to proxy_balancer.conf")
                                add_to_balancer(pod_ip, tomcat_port)
                                tomcats.append(pod_ip)
                                reload_httpd()
                            time.sleep(1)
                            ++i
                        if i == 10:
                            print(
                                "Can't get a response from tomcat, check pod "+pod_name)
                    # Stops repeating error message while pod is being deleted, after removing it from tomcats
                    elif pod_ip not in tomcats and \
                            (pod.metadata.deletion_grace_period_seconds is not None or
                             pod.metadata.deletion_timestamp is not None):
                        pass
                    else:
                        print("Tomcat on pod "+pod_name+" is offline.")
                else:
                    print("Pod "+pod_name+" has no IP yet.")
            else:
                print("Pod status elements for pod " +
                      pod_name+" are not initialized yet.")


if __name__ == "__main__":
    main()
