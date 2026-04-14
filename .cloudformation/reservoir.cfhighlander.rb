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

    Component name: 'app', template: 'ecs-service@2.18.0', render:Inline do
      parameter name: 'VPCId', value: FnImportValue(FnSub("${EnvironmentName}-vpc-VPCId"))
      parameter name: 'SubnetIds', value: FnImportValue(FnSub("${EnvironmentName}-vpc-ComputeSubnets"))
      parameter name: 'EcsCluster', value: FnImportValue(FnSub("${EnvironmentName}-ecs-EcsCluster"))

      parameter name: 'DesiredCount', value: Ref('DesiredCount')
      parameter name: 'MinCount', value: Ref('MinCount')
      parameter name: 'MaxCount', value: Ref('MaxCount')
      parameter name: 'Cpu', value: Ref('Cpu')
      parameter name: 'Memory', value: Ref('Memory')
      parameter name: 'EnableScaling', value: Ref('EnableScaling')
      parameter name: 'appVersion', value: component_version
    end

  end
