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
      mountPath: /root/.docker-secret
      readOnly: true
  - name: jnlp
    volumeMounts:
    - name: github-key-volume
      mountPath: /home/jenkins/.ssh-secret
      readOnly: true
  volumes:
  - name: docker-config
    secret:
      secretName: docker-credentials
      items:
      - key: .dockerconfigjson
        path: config.json
  - name: github-key-volume
    secret:
      secretName: github-deploy-key
      defaultMode: 256
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
                        # Prepare Docker credentials in writable directory
                        mkdir -p /root/.docker
                        if [ -f /root/.docker-secret/config.json ]; then
                            cp /root/.docker-secret/config.json /root/.docker/config.json
                        fi

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
                    if git diff --cached --quiet; then
                        echo "No manifest changes to commit."
                    else
                        git commit -m "ci(gitops): update image tag to ${IMAGE_TAG} [skip ci]"
                        if [ -f /home/jenkins/.ssh-secret/id_rsa ]; then
                            mkdir -p /home/jenkins/.ssh
                            cp /home/jenkins/.ssh-secret/id_rsa /home/jenkins/.ssh/id_rsa
                            chmod 600 /home/jenkins/.ssh/id_rsa
                            export GIT_SSH_COMMAND="ssh -i /home/jenkins/.ssh/id_rsa -o StrictHostKeyChecking=no"
                            git push git@github.com:Siddartha-02/payment-gateway-microservices.git HEAD:main
                            echo "Successfully pushed manifest updates to GitHub!"
                        else
                            echo "Warning: GitHub deploy key not found at /home/jenkins/.ssh-secret/id_rsa"
                        fi
                    fi
                '''
            }
        }
    }

    post {
        success {
            echo "=========================================================="
            echo "  ✔ BUILD & PUSH SUCCEEDED!"
            echo "  Docker Hub images pushed: ${DOCKER_HUB_USER}/*:${IMAGE_TAG}"
            echo "  ArgoCD manifests updated and pushed to GitHub."
            echo "  You can now trigger manual Sync in ArgoCD!"
            echo "=========================================================="
        }
        failure {
            echo "❌ Pipeline failed! Please inspect the logs above."
        }
    }
}
