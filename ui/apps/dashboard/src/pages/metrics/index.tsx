import { useState, useEffect } from 'react';
import { Layout, Button, Card, Space, Typography, Select, Input, message } from 'antd';
import Panel from '@/components/panel';
import { GetMetricsInfo, GetMetricsData } from '@/services/metrics';
import Diagram from '@/pages/metrics/diagram';

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
  const [selectedOption, setSelectedOption] = useState<string>(() => {
    return localStorage.getItem('selectedOption') || '';
  });
  const [searchMetric, setSearchMetric] = useState<string>('');
  const [selectedMetric, setSelectedMetric] = useState<Metric | null>(() => {
    const savedMetric = localStorage.getItem('selectedMetric');
    return savedMetric ? JSON.parse(savedMetric) : null;
  });
  const [selectedPod, setSelectedPod] = useState<string>(() => {
    return localStorage.getItem('selectedPod') || '';
  });
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [pods, setPods] = useState<PodOption[]>([]);
  const [syncStatus, setSyncStatus] = useState<'idle' | 'success' | 'error'>('idle');
  const [lastUpdated, setLastUpdated] = useState<number>(0); // New state

  const options = [
    'karmada-scheduler-estimator-member1',
    'karmada-scheduler-estimator-member2',
    'karmada-scheduler-estimator-member3',
    'karmada-controller-manager',
    'karmada-agent',
    'karmada-scheduler',
  ];

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

      const fetchedPods = Object.keys(data).map(podName => ({
        id: podName,
        name: podName.replace(/_/g, ' ')
      }));
      setPods(fetchedPods);
      console.log("Processed pods:", fetchedPods);
    } catch (error) {
      console.error('Failed to fetch metrics:', error);
      message.error('Failed to fetch metrics');
    }
  };

  useEffect(() => {
    fetchMetrics();
  }, [selectedOption]);

  useEffect(() => {
    localStorage.setItem('selectedOption', selectedOption);
  }, [selectedOption]);

  useEffect(() => {
    localStorage.setItem('selectedPod', selectedPod);
  }, [selectedPod]);

  useEffect(() => {
    if (selectedMetric) {
      localStorage.setItem('selectedMetric', JSON.stringify(selectedMetric));
    } else {
      localStorage.removeItem('selectedMetric');
    }
  }, [selectedMetric]);

  const filteredMetrics = metrics.filter(metric => 
    metric.name.toLowerCase().includes(searchMetric.toLowerCase())
  );

  const handleMetricSelect = (metric: Metric) => {
    setSelectedMetric(metric);
    setSearchMetric('');
  };

  const handleOptionChange = (value: string) => {
    console.log(`Changing component from ${selectedOption} to ${value}`);
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

  const handleSync = async () => {
    try {
      if (selectedOption === '') {
        message.error('Please select a component');
        return;
      } else {    
        const response = await GetMetricsData(selectedOption);
        console.log("Sync response:", response);

        if (response.status === 200) {
          setSyncStatus('success');
          message.success('Sync successful!');
          await fetchMetrics(); // Ensure fetchMetrics completes
          setLastUpdated(Date.now()); // Update lastUpdated
          setTimeout(() => setSyncStatus('idle'), 5000); // Reset status after 5 seconds
        } else {
          throw new Error('Sync failed with status: ' + response.status);
        }
      }
    } catch (error) {
      setSyncStatus('error');
      message.error('Sync failed: ' + (error instanceof Error ? error.message : 'due to an unknown error'));
      setTimeout(() => setSyncStatus('idle'), 5000); // Reset status after 5 seconds
    }
  };

  return (
    <Panel>
      <Layout style={{ height: '100vh' }}>
        <Sider width={800} style={{ background: '#fff', padding: '16px' }}>
          <Space direction="vertical" style={{ width: '100%' }} size="small">
            {/* Selection Controls */}
            <div style={{ display: 'flex' }}>
              <Select
                allowClear
                key={`component-select`} // Optional: unique key if needed
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
                  allowClear
                  key={`pod-select-${selectedOption}`} // Unique key based on selectedOption
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
                  style={{ width: '200px' }}
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
                      transition: 'background-color 0.3s',
                    }} 
                    onClick={() => handleMetricSelect(metric)}
                    onMouseEnter={(e) => e.currentTarget.style.backgroundColor = '#f5f5f5'}
                    onMouseLeave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
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

            <Diagram
              activeTab={activeTab}
              setActiveTab={setActiveTab}
              componentName={selectedOption}
              podsName={selectedPod}
              metricName={selectedMetric ? selectedMetric.name : ''}
              lastUpdated={lastUpdated} // Pass the new prop
            />
          </Space>
        </Sider>
        
        <Content style={{ padding: '16px', background: '#fff' }}>
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '16px' }}>
            <Space>
              {syncStatus === 'success' ? (
                <Button style={{ color: 'green' }} onClick={handleSync}>
                  Sync Successful
                </Button>
              ) : syncStatus === 'error' ? (
                <Button style={{ color: 'red' }} onClick={handleSync}>
                  Sync Failed - Retry?
                </Button>
              ) : (
                <Button onClick={handleSync}>Sync db</Button>
              )}
              {/* <Button danger>Delete</Button> */}
            </Space>
          </div>
        </Content>
      </Layout>
    </Panel>
  );
}
