pipeline {
    agent {
        kubernetes {
            yaml '''
apiVersion: v1
kind: Pod
metadata:
  labels:
    jenkins-agent: payment-gateway-builder
spec:
  containers:
  - name: dind
    image: docker:dind
    securityContext:
      privileged: true
    env:
    - name: DOCKER_TLS_CERTDIR
      value: ""
  - name: docker
    image: docker:cli
    command:
    - sleep
    args:
    - 9999999
    env:
    - name: DOCKER_HOST
      value: tcp://localhost:2375
    volumeMounts:
    - name: docker-config
      mountPath: /root/.docker
  volumes:
  - name: docker-config
    secret:
      secretName: docker-credentials
      items:
      - key: .dockerconfigjson
        path: config.json
'''
        }
    }

    environment {
        DOCKER_HUB_USER = 'siddartha02'
        IMAGE_TAG       = "${BUILD_NUMBER}"
    }

    stages {
        stage('1. Checkout Source') {
            steps {
                echo "Fetching latest source code from GitHub..."
                checkout scm
            }
        }

        stage('2. Build & Push Docker Images') {
            steps {
                container('docker') {
                    echo "Building and pushing container images with tag: ${IMAGE_TAG} and latest..."
                    sh '''
                        echo "Building Merchant Service..."
                        docker build -t ${DOCKER_HUB_USER}/payment-gateway-merchant-service:${IMAGE_TAG} \
                                     -t ${DOCKER_HUB_USER}/payment-gateway-merchant-service:latest \
                                     services/merchant-service
                        docker push ${DOCKER_HUB_USER}/payment-gateway-merchant-service:${IMAGE_TAG}
                        docker push ${DOCKER_HUB_USER}/payment-gateway-merchant-service:latest

                        echo "Building Payment Service..."
                        docker build -t ${DOCKER_HUB_USER}/payment-gateway-payment-service:${IMAGE_TAG} \
                                     -t ${DOCKER_HUB_USER}/payment-gateway-payment-service:latest \
                                     services/payment-service
                        docker push ${DOCKER_HUB_USER}/payment-gateway-payment-service:${IMAGE_TAG}
                        docker push ${DOCKER_HUB_USER}/payment-gateway-payment-service:latest

                        echo "Building Ledger Service..."
                        docker build -t ${DOCKER_HUB_USER}/payment-gateway-ledger-service:${IMAGE_TAG} \
                                     -t ${DOCKER_HUB_USER}/payment-gateway-ledger-service:latest \
                                     services/ledger-service
                        docker push ${DOCKER_HUB_USER}/payment-gateway-ledger-service:${IMAGE_TAG}
                        docker push ${DOCKER_HUB_USER}/payment-gateway-ledger-service:latest

                        echo "Building Dashboard UI..."
                        docker build -t ${DOCKER_HUB_USER}/payment-gateway-dashboard:${IMAGE_TAG} \
                                     -t ${DOCKER_HUB_USER}/payment-gateway-dashboard:latest \
                                     services/dashboard
                        docker push ${DOCKER_HUB_USER}/payment-gateway-dashboard:${IMAGE_TAG}
                        docker push ${DOCKER_HUB_USER}/payment-gateway-dashboard:latest
                    '''
                }
            }
        }

        stage('3. Update GitOps Manifests') {
            steps {
                echo "Updating K8s manifests with new image tag (${IMAGE_TAG})..."
                sh '''
                    sed -i "s|image: .*payment-gateway-merchant-service:.*|image: ${DOCKER_HUB_USER}/payment-gateway-merchant-service:${IMAGE_TAG}|g" k8s/merchant-service.yaml
                    sed -i "s|image: .*payment-gateway-payment-service:.*|image: ${DOCKER_HUB_USER}/payment-gateway-payment-service:${IMAGE_TAG}|g" k8s/payment-service.yaml
                    sed -i "s|image: .*payment-gateway-ledger-service:.*|image: ${DOCKER_HUB_USER}/payment-gateway-ledger-service:${IMAGE_TAG}|g" k8s/ledger-service.yaml
                    sed -i "s|image: .*payment-gateway-dashboard:.*|image: ${DOCKER_HUB_USER}/payment-gateway-dashboard:${IMAGE_TAG}|g" k8s/dashboard.yaml
                    
                    git config user.name "jenkins-bot"
                    git config user.email "jenkins@siddhu.ddns.net"
                    
                    git add k8s/*.yaml
                    git diff --cached --quiet || git commit -m "ci(gitops): update image tag to ${IMAGE_TAG} [skip ci]"
                '''
            }
        }
    }

    post {
        success {
            echo "=========================================================="
            echo "  ✔ BUILD & PUSH SUCCEEDED!"
            echo "  Docker Hub images pushed: ${DOCKER_HUB_USER}/*:${IMAGE_TAG}"
            echo "  ArgoCD will now detect the changes for manual sync."
            echo "=========================================================="
        }
        failure {
            echo "❌ Pipeline failed! Please inspect the logs above."
        }
    }
}
