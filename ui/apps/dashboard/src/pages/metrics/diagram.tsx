import React, { useEffect, useState } from 'react';
import { Tabs, Card, Modal, Button } from 'antd';
import { GetMetricsDetails, MetricDetailsResponse } from '@/services/metrics';

interface MetricTabsProps {
  activeTab: string;
  setActiveTab: (key: string) => void;
  componentName: string;
  podsName: string;
  metricName: string;
}

const Diagram: React.FC<MetricTabsProps> = ({ activeTab, setActiveTab, componentName, podsName, metricName }) => {
  const [visible, setVisible] = useState(false);
  const [logs, setLogs] = useState<MetricDetailsResponse | null>(null);

  const getLogs = async (componentName: string, podsName: string, metricName: string) => {
    if (!componentName || !podsName || !metricName) {
      console.error('Missing parameters for fetching metrics details');
      return; 
    }

    try {
      const details = await GetMetricsDetails(componentName, podsName, metricName);
      console.log('Metrics Details:', details);
      setLogs(details);
    } catch (error) {
      console.error('Failed to fetch metrics details:', error);
      setLogs(null); // Optionally clear logs on error
    }
  };
  
  useEffect(() => {
    if (componentName && podsName && metricName) {
      getLogs(componentName, podsName, metricName);
    } else {
      setLogs(null);
    }
  }, [componentName, podsName, metricName]); // Removed lastUpdated

  const showModal = () => {
    setVisible(true);
  };

  const handleDelete = (range: string) => {
    console.log(`Deleting range: ${range}`);
    // Implement deletion logic here if needed
  };

  return (
    <div>
      <Button type="primary" onClick={showModal} style={{ float: 'right', marginBottom: '16px' }}>
        View Date-Time Ranges
      </Button>
      <Modal
        title="Time Ranges"
        visible={visible}
        onOk={() => setVisible(false)}
        onCancel={() => setVisible(false)}
        footer={null}
      >
        <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
          {logs && logs.details
            ? Object.keys(logs.details).map((range, index) => (
                <div key={index} style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '16px' }}>
                  <span>{range}</span>
                  <Button type="link" onClick={() => handleDelete(range)}>
                    Delete
                  </Button>
                </div>
              ))
            : <p>No time ranges available</p>
          }
        </div>
      </Modal>
      <Tabs
        activeKey={activeTab}
        onChange={key => setActiveTab(key.toString())}
        style={{ marginRight: '24px' }}
        items={[
          {
            key: 'graph',
            label: 'Graph',
            children: (
              <div>
                <b>Here should be the histogram</b>
                <Card title="Logs" style={{ marginTop: '16px' }}>
                  <div style={{ height: '200px', background: '#f5f5f5', overflowY: 'auto' }}>
                    {logs ? JSON.stringify(logs, null, 2) : 'No logs available'}
                  </div>
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
    </div>
  );
};

export default Diagram;
