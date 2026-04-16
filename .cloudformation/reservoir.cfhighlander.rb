CfhighlanderTemplate do

    Description "Boddle Learning - (#{component_name}@#{component_version})"

    Parameters do
      ComponentParam 'DesiredCount', 1
      ComponentParam 'MinCount', 1
      ComponentParam 'MaxCount', 1
      ComponentParam 'EnableScaling', 'false'
      ComponentParam 'Cpu', '512'
      ComponentParam 'Memory', '1024'
    end

    Condition('DontSetDesireCount', FnEquals(Ref(:DesiredCount), '-1'))

    # fargate-v2: when app.config.yaml uses a targetgroup *array*, the child stack expects
    # one `{listener}Listener` parameter per target group (with rules), not `Listener` / `LoadBalancer`.
    # Parent VPC/ALB stacks must export the internal ALB listener ARN and LB security group using
    # the same names you pass to FnImportValue below (adjust Fn::Sub strings to match real exports).
    Component name: 'app', template: 'fargate-v2@0.7.5', render:Inline do
      parameter name: 'VPCId', value: FnImportValue(FnSub("${EnvironmentName}-vpc-VPCId"))
      parameter name: 'SubnetIds', value: FnSplit(',', FnImportValue(FnSub("${EnvironmentName}-vpc-ComputeSubnets")))
      parameter name: 'EcsCluster', value: FnImportValue(FnSub("${EnvironmentName}-ecs-EcsCluster"))
      parameter name: 'httpsListener', value: FnImportValue(FnSub("${EnvironmentName}-alb-httpsListener"))
      parameter name: 'internalListener', value: FnImportValue(FnSub("${EnvironmentName}-internalalb-httpsListener"))
      parameter name: 'LoadBalancerSecurityGroup', value: FnImportValue(FnSub("${EnvironmentName}-alb-SecurityGroupLoadBalancer"))
      parameter name: 'InternalLoadBalancerSecurityGroup', value: FnImportValue(FnSub("${EnvironmentName}-internalalb-SecurityGroupLoadBalancer"))
      parameter name: 'DnsDomain', value: FnSub("${EnvironmentName}.env.boddlelearning.com")
      parameter name: 'DesiredCount', value: FnIf('DontSetDesireCount', Ref('AWS::NoValue'), Ref('DesiredCount'))
      parameter name: 'appScalingMin', value: Ref('MinCount')
      parameter name: 'appScalingMax', value: Ref('MaxCount')
      parameter name: 'MinimumHealthyPercent', value: 100
      parameter name: 'MaximumPercent', value: 200
      parameter name: 'Cpu', value: Ref('Cpu')
      parameter name: 'Memory', value: Ref('Memory')
      parameter name: 'EnableScaling', value: Ref('EnableScaling')
      parameter name: 'appTaskVersion', value: component_version
    end

  end
