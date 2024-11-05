import { useState, useEffect } from 'react';
import { Layout, Button, Tabs, Card, Space, Typography, Select, Input, Table } from 'antd';
import Panel from '@/components/panel';
import { GetMetricsDetails, GetMetricsInfo } from '@/services/metrics';

const { Sider, Content } = Layout;
const { Text } = Typography;
const { Option } = Select;

interface Metric {
  name: string;
  type: string;
  help: string;
}

interface PodOption {
  id: string;
  name: string;
}

export default function Component() {
  const [activeTab, setActiveTab] = useState<string>('graph');
  const [selectedOption, setSelectedOption] = useState<string>('');
  const [searchMetric, setSearchMetric] = useState<string>('');
  const [selectedMetric, setSelectedMetric] = useState<Metric | null>(null);
  const [selectedPod, setSelectedPod] = useState<string>('');
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [pods, setPods] = useState<PodOption[]>([]);

  const options = [
    'karmada-scheduler-estimator-member1',
    'karmada-scheduler-estimator-member2',
    'karmada-scheduler-estimator-member3',
    'karmada-controller-manager',
    'karmada-agent',
    'karmada-scheduler',
  ];

  const graphData = [
    { time: '00:00', value: 30 },
    { time: '04:00', value: 50 },
    { time: '08:00', value: 80 },

  ];

  const columns = [
    { title: 'Time', dataIndex: 'time', key: 'time' },
    { title: 'Value', dataIndex: 'value', key: 'value' },
  ];

  useEffect(() => {
    const fetchMetrics = async () => {
      if (!selectedOption) return;
      
      console.log("Fetching metrics for option:", selectedOption);
      try {
        const data = await GetMetricsInfo(selectedOption, 'metricsdetails');
        console.log("Metrics data received:", data);

        const fetchedMetrics = Object.entries(data).flatMap(([clusterName, clusterMetrics]) =>
          Object.entries(clusterMetrics).map(([metricName, metricInfo]) => ({
            name: metricName,
            type: metricInfo.type,
            help: metricInfo.help
          }))
        );
        setMetrics(fetchedMetrics);
        console.log("Processed metrics:", fetchedMetrics);

        const fetchedPods = Object.keys(data).map(clusterName => ({
          id: clusterName,
          name: clusterName.replace(/_/g, ' ')
        }));
        setPods(fetchedPods);
        console.log("Processed pods:", fetchedPods);
      } catch (error) {
        console.error('Failed to fetch metrics:', error);
      }
    };

    fetchMetrics();
  }, [selectedOption]);

  const filteredMetrics = metrics.filter(metric => 
    metric.name.toLowerCase().includes(searchMetric.toLowerCase())
  );

  const handleMetricSelect = (metric: Metric) => {
    setSelectedMetric(metric);
    setSearchMetric('');
  };

  const handleOptionChange = (value: string) => {
    setSelectedOption(value);
    setSelectedPod('');
    setSearchMetric('');
    setSelectedMetric(null);
  };

  const handlePodChange = (value: string) => {
    setSelectedPod(value);
    setSearchMetric('');
    setSelectedMetric(null);
  };

  return (
    <Panel>
      <Layout style={{ height: '100vh' }}>
        <Sider width={800} style={{ background: '#fff', padding: '16px' }}>
          <Space direction="vertical" style={{ width: '100%' }} size="small">
            {/* Selection Controls */}
            <div style={{ display: 'flex'}}>
              <Select
                style={{ width: '200px' }}
                placeholder="Select Option"
                value={selectedOption}
                onChange={handleOptionChange}
              >
                {options.map(option => (
                  <Option key={option} value={option}>{option}</Option>
                ))}
              </Select>

              {selectedOption && (
                <Select
                  style={{ width: '300px' }}
                  placeholder="Select Pod"
                  value={selectedPod}
                  onChange={handlePodChange}
                >
                  {pods.map(pod => (
                    <Option key={pod.id} value={pod.id}>{pod.name}</Option>
                  ))}
                </Select>
              )}

              {selectedPod && (
                <Input
                  style={{  width: '200px'}}
                  placeholder="Search metrics"
                  value={selectedMetric ? selectedMetric.name : searchMetric}
                  onChange={(e) => setSearchMetric(e.target.value)}
                />
              )}
            </div>

            {/* Metrics List */}
            {searchMetric && (
              <Card size="small" title="Matching Metrics">
                {filteredMetrics.map(metric => (
                  <div 
                    key={metric.name} 
                    style={{ 
                      marginBottom: '8px', 
                      cursor: 'pointer',
                      padding: '4px',
                      borderRadius: '4px',
                      '&:hover': { backgroundColor: '#f5f5f5' }
                    } as React.CSSProperties} 
                    onClick={() => handleMetricSelect(metric)}
                  >
                    <Text>{metric.name}</Text>
                    <br />
                    <Text type="secondary" style={{ fontSize: '12px' }}>TYPE: {metric.type}</Text>
                  </div>
                ))}
              </Card>
            )}
          
            {/* Metric Details */}
            {selectedMetric && (
              <Card size="small" title="Metric Details">
                <div>
                  <Text>Type: {selectedMetric.type}</Text>
                  <br />
                  <Text>Help: {selectedMetric.help}</Text>  
                </div>
              </Card>
            )}

            <Tabs
              activeKey={activeTab}
              onChange={key => setActiveTab(key.toString())}
              items={[
                {
                  key: 'graph',
                  label: 'Graph',
                  children: (
                    <div>
                      <Table dataSource={graphData} columns={columns} pagination={false} />
                      <Card title="Logs" style={{ marginTop: '16px' }}>
                        <div style={{ height: '200px', background: '#f5f5f5' }}></div>
                      </Card>
                    </div>
                  ),
                },
                {
                  key: 'query',
                  label: 'Query',
                  children: <div style={{ padding: '24px' }}>Query content will go here</div>,
                },
              ]}
            />
          </Space>
        </Sider>
        
        <Content style={{ padding: '16px', background: '#fff' }}>
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '16px' }}>
            <Space>
              <Button>Sync</Button>
              <Button danger>Delete</Button>
            </Space>
          </div>
        </Content>
      </Layout>
    </Panel>
  );
}