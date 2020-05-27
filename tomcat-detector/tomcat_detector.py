from kubernetes import client, config, watch
import requests
import json


def checkIfPodIsTomcat(pod):
    if pod.spec.containers[0].image == "tomcat:9.0" and \
            pod.metadata.labels['category'] == "test-dynamic-reverse-proxy":
        return True
    return False


def main():
    config.load_incluster_config()

    v1 = client.CoreV1Api()

    w = watch.Watch()

    for event in w.stream(v1.list_pod_for_all_namespaces):
        # type = event['type']
        pod = event['object']
        if checkIfPodIsTomcat(pod):
            print("Namespace: %s Pod Name: %s Event: %s" %
              (pod.metadata.namespace, pod.metadata.name, event['type']))
            if pod.status.conditions is not None and pod.status.container_statuses is not None:
                if pod.status.pod_ip is not None:
                    if pod.status.conditions[0].type == "Initialized" and \
                            pod.status.conditions[1].status == "True" and \
                            pod.status.conditions[2].status == "True" and \
                            pod.status.container_statuses[0].image == "tomcat:9.0" and \
                            pod.status.container_statuses[0].state.running is not None and \
                            pod.metadata.deletion_grace_period_seconds is None and \
                            pod.metadata.deletion_timestamp is None:
                        ret = requests.get(
                            "http://"+pod.status.pod_ip+":8080")
                        if ret.status_code == 200:
                            print("Tomcat is online and working")
                    else:
                        print("Tomcat is not online yet")
                        # print(pod.status.conditions[0].type == "Initialized")
                        # print(pod.status.conditions[1].status == "True")
                        # print(pod.status.conditions[2].status == "True")
                        # print(
                        #     pod.status.container_statuses[0].image == "tomcat:v5.3.0-pprokopi")
                        # print(
                        #     pod.status.container_statuses[0].state.running is not None)
                else:
                    print("Pod has no IP yet.")
            else:
                print("Pod status elements are not initialized yet")
            # print(pod)


if __name__ == "__main__":
    main()
