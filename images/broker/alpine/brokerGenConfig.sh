#!/bin/bash

# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

BROKER_CONFIG_FILE="$ROCKETMQ_HOME/conf/broker.conf"
BROKER_CONFIG_MOUNT_FILE="$ROCKETMQ_HOME/conf/broker-common.conf"
BROKER_CONFIG_PLAIN_ACL="$ROCKETMQ_HOME/conf/plain_acl.yml"
BROKER_CONFIG_TOOLS="$ROCKETMQ_HOME/conf/tools.yml"

mkdir -p $ROCKETMQ_HOME/conf/acl

function create_config() {
    rm -f $BROKER_CONFIG_FILE
    echo "Creating broker configuration."
    cat $BROKER_CONFIG_MOUNT_FILE > $BROKER_CONFIG_FILE
    # Make config file plain_acl.yml
    sed -n '/config plain_acl.yml start/,/config plain_acl.yml end/p' $BROKER_CONFIG_FILE > $BROKER_CONFIG_PLAIN_ACL
#    sed -i '/config plain_acl.yml/d' $BROKER_CONFIG_PLAIN_ACL
    sed -i '/config plain_acl.yml start/,/config plain_acl.yml end/d' $BROKER_CONFIG_FILE

    # Make config file tools.yml
    sed -n '/config tools_yml start/,/config tools_yml end/p' $BROKER_CONFIG_FILE > $BROKER_CONFIG_TOOLS
    sed -i '/config tools_yml start/,/config tools_yml end/d' $BROKER_CONFIG_FILE

    # Remove brokerClusterName, brokerName, brokerId if configured
    sed -i -e '/brokerClusterName/d;/brokerName/d;/brokerId/d' $BROKER_CONFIG_FILE
    echo -e >> $BROKER_CONFIG_FILE
    echo "brokerClusterName=$BROKER_CLUSTER_NAME" >> $BROKER_CONFIG_FILE
    echo "brokerName=$BROKER_NAME" >> $BROKER_CONFIG_FILE
    echo "brokerId=$BROKER_ID" >> $BROKER_CONFIG_FILE
    echo "brokerIP1=`hostname -i`" >> $BROKER_CONFIG_FILE
    if [ $BROKER_ID != 0 ]; then
        sed -i 's/brokerRole=.*/brokerRole=SLAVE/g' $BROKER_CONFIG_FILE
    fi
}

create_config
cat $BROKER_CONFIG_FILE
