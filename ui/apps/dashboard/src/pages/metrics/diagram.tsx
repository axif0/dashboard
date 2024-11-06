import React, { useEffect, useState } from 'react';
import { Tabs, Table, Card, Modal, Button, message } from 'antd';
import {GetMetricsDetails, MetricDetailsResponse } from '@/services/metrics';
interface MetricTabsProps {
  activeTab: string;
  setActiveTab: (key: string) => void;
  componentName: string;
  podsName: string;
  metricName: string;
}
const graphData = [
    { time: '00:00', value: 30 },
    { time: '04:00', value: 50 },
    { time: '08:00', value: 80 },

  ];

  const columns = [
    { title: 'Time', dataIndex: 'time', key: 'time' },
    { title: 'Value', dataIndex: 'value', key: 'value' },
  ];

  
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
      }
    }
 
    useEffect(() => {
      if (componentName && podsName && metricName) {
        getLogs(componentName, podsName, metricName);
      } else {
  
        setLogs(null);
      }
    }, [componentName, podsName, metricName]);  

  // Demo date-time ranges
  const dateTimeRanges = [
    '2024-11-05T22:59:52+07:00',
    '2024-11-06T23:00:53+08:00',
    '2024-11-07T00:01:54+09:00',
    '2024-11-07T01:02:55+10:00',
    '2024-11-07T02:03:56+11:00'
  ];

  const showModal = () => {
    setVisible(true);
  };

  const handleDelete = (range: string) => {
 
    console.log(`Deleting range: ${range}`);
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
        {dateTimeRanges.map((range, index) => (
          <div key={index} style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '16px' }}>
            <span>{range}</span>
            <Button type="link" onClick={() => handleDelete(range)}>
              Delete
            </Button>
          </div>
        ))}
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