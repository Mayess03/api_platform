FROM wso2/wso2am-acp:4.7.0

ADD --chown=wso2carbon:wso2 \
  https://repo1.maven.org/maven2/com/mysql/mysql-connector-j/8.4.0/mysql-connector-j-8.4.0.jar \
  /home/wso2carbon/wso2am-acp-4.7.0/repository/components/lib/

# WSO2 Custom Gateway Agent - MuleSoft Anypoint / Flex Gateway
COPY --chown=wso2carbon:wso2 \
  agents/custom.gw.manager-1.0.0-SNAPSHOT.jar \
  /home/wso2carbon/wso2am-acp-4.7.0/repository/components/dropins/