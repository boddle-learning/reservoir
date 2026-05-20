CfhighlanderTemplate do

    Description "Boddle Learning - (#{component_name}@#{component_version})"

    Parameters do
      # Defaults sized for ~5,000 req/min of live auth traffic. See
      # docs/CAPACITY_PLANNING.md. Single-task / no-scaling defaults
      # contributed to the 2026-05-19 outage and must not be the default
      # for any environment that takes live traffic. MaxCount=8 matches
      # the prod1 override and leaves headroom for the school-day peak
      # (incident relief required ~16; an environment expecting that
      # load should set its own override).
      ComponentParam 'DesiredCount', 2
      ComponentParam 'MinCount', 2
      ComponentParam 'MaxCount', 8
      ComponentParam 'EnableScaling', 'true'
      ComponentParam 'Cpu', '4096'     # 4 vCPU
      ComponentParam 'Memory', '8192'  # 8 GB (Fargate minimum at 4 vCPU)
    end

    Condition('DontSetDesireCount', FnEquals(Ref(:DesiredCount), '-1'))

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
