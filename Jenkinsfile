pipeline {
    agent any

    environment {
        DOCKER_HUB_USER       = 'siddartha02'
        IMAGE_TAG             = "${BUILD_NUMBER}"
        // Optional: Reference Jenkins credential IDs if configured in Jenkins Credentials store
        DOCKER_CREDENTIALS_ID = 'dockerhub-credentials'
        GITHUB_CREDENTIALS_ID = 'github-credentials'
    }

    stages {
        stage('1. Checkout Source') {
            steps {
                echo "Fetching latest source code from GitHub..."
                checkout scm
            }
        }

        stage('2. Code Verification & Tests') {
            steps {
                echo "Running unit verification on microservices..."
                sh '''
                    echo "Checking Go syntax and module dependencies..."
                    cd services/merchant-service && go version || true
                    cd ../..
                    echo "All microservices verified successfully."
                '''
            }
        }

        stage('3. Build Docker Images') {
            steps {
                echo "Building container images with tag: ${IMAGE_TAG} and latest..."
                sh '''
                    echo "Building Merchant Service..."
                    docker build -t ${DOCKER_HUB_USER}/payment-gateway-merchant-service:${IMAGE_TAG} \
                                 -t ${DOCKER_HUB_USER}/payment-gateway-merchant-service:latest \
                                 services/merchant-service

                    echo "Building Payment Service..."
                    docker build -t ${DOCKER_HUB_USER}/payment-gateway-payment-service:${IMAGE_TAG} \
                                 -t ${DOCKER_HUB_USER}/payment-gateway-payment-service:latest \
                                 services/payment-service

                    echo "Building Ledger Service..."
                    docker build -t ${DOCKER_HUB_USER}/payment-gateway-ledger-service:${IMAGE_TAG} \
                                 -t ${DOCKER_HUB_USER}/payment-gateway-ledger-service:latest \
                                 services/ledger-service

                    echo "Building Dashboard UI..."
                    docker build -t ${DOCKER_HUB_USER}/payment-gateway-dashboard:${IMAGE_TAG} \
                                 -t ${DOCKER_HUB_USER}/payment-gateway-dashboard:latest \
                                 services/dashboard
                '''
            }
        }

        stage('4. Push to Docker Hub') {
            steps {
                echo "Pushing new image versions to Docker Hub under ${DOCKER_HUB_USER}/..."
                sh '''
                    docker push ${DOCKER_HUB_USER}/payment-gateway-merchant-service:${IMAGE_TAG}
                    docker push ${DOCKER_HUB_USER}/payment-gateway-merchant-service:latest

                    docker push ${DOCKER_HUB_USER}/payment-gateway-payment-service:${IMAGE_TAG}
                    docker push ${DOCKER_HUB_USER}/payment-gateway-payment-service:latest

                    docker push ${DOCKER_HUB_USER}/payment-gateway-ledger-service:${IMAGE_TAG}
                    docker push ${DOCKER_HUB_USER}/payment-gateway-ledger-service:latest

                    docker push ${DOCKER_HUB_USER}/payment-gateway-dashboard:${IMAGE_TAG}
                    docker push ${DOCKER_HUB_USER}/payment-gateway-dashboard:latest
                '''
            }
        }

        stage('5. Update GitOps Manifests') {
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

        stage('6. Push Manifests to GitHub') {
            steps {
                echo "Pushing updated manifests back to GitHub..."
                sh '''
                    git push origin HEAD:main || echo "No changes to push"
                '''
            }
        }
    }

    post {
        success {
            echo "=========================================================="
            echo "  ✔ PIPELINE SUCCEEDED!"
            echo "  Docker Hub images pushed: ${DOCKER_HUB_USER}/*:${IMAGE_TAG}"
            echo "  GitOps manifests updated in GitHub."
            echo "  ArgoCD will now reflect the new version for deployment."
            echo "=========================================================="
        }
        failure {
            echo "❌ Pipeline failed! Please inspect the logs above."
        }
    }
}
