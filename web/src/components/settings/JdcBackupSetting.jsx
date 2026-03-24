import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Radio,
  Spin,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, toBoolean } from '../../helpers';

const { Text } = Typography;

const DEFAULT_INPUTS = {
  'jdc_backup.backup_enabled': true,
  'jdc_backup.backup_time': '00:00',
  'jdc_backup.backup_dir': '',
  'jdc_backup.retain_count': 7,
  'jdc_backup.cleanup_enabled': true,
  'jdc_backup.log_retention_days': 30,
  'jdc_backup.quota_data_retention_days': 30,
  'jdc_backup.include_logs_in_export': false,
};

export default function JdcBackupSetting() {
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(DEFAULT_INPUTS);
  const [records, setRecords] = useState([]);
  const [importFile, setImportFile] = useState(null);
  const formRef = useRef(null);

  const load = async () => {
    setLoading(true);
    try {
      const [optionsRes, recordsRes] = await Promise.all([
        API.get('/api/option/'),
        API.get('/api/jdc/backup/'),
      ]);
      if (optionsRes.data.success) {
        const nextInputs = { ...DEFAULT_INPUTS };
        optionsRes.data.data.forEach((item) => {
          if (!(item.key in nextInputs)) return;
          if (typeof nextInputs[item.key] === 'boolean') {
            nextInputs[item.key] = toBoolean(item.value);
          } else if (typeof nextInputs[item.key] === 'number') {
            nextInputs[item.key] = Number(item.value);
          } else {
            nextInputs[item.key] = item.value;
          }
        });
        setInputs(nextInputs);
        if (formRef.current) {
          formRef.current.setValues(nextInputs);
        }
      }
      if (recordsRes.data.success) {
        setRecords(recordsRes.data.data || []);
      }
    } catch (error) {
      showError('加载 JDC 备份设置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const updateField = (key) => (value) => {
    setInputs((prev) => ({ ...prev, [key]: value }));
  };

  const save = async () => {
    setLoading(true);
    try {
      await Promise.all(
        Object.entries(inputs).map(([key, value]) =>
          API.put('/api/option/', {
            key,
            value: typeof value === 'boolean' ? String(value) : String(value ?? ''),
          }),
        ),
      );
      showSuccess('JDC 备份设置已保存');
      load();
    } catch (error) {
      showError('保存失败');
    } finally {
      setLoading(false);
    }
  };

  const createBackup = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/jdc/backup/create');
      if (res.data.success) {
        showSuccess('备份已创建');
        load();
      }
    } catch (error) {
      showError('创建备份失败');
    } finally {
      setLoading(false);
    }
  };

  const cleanupLogs = async () => {
    setLoading(true);
    try {
      const res = await API.delete('/api/jdc/backup/cleanup_logs');
      if (res.data.success) {
        const data = res.data.data || {};
        showSuccess(`数据库日志已清理，日志 ${data.deleted_logs || 0} 条，quota_data ${data.deleted_quota_data || 0} 条`);
      }
    } catch (error) {
      showError('清理数据库日志失败');
    } finally {
      setLoading(false);
    }
  };

  const restoreBackup = async (id) => {
    setLoading(true);
    try {
      const res = await API.post(`/api/jdc/backup/restore/${id}`);
      if (res.data.success) {
        showSuccess('数据库已还原，请刷新页面确认最新数据');
      }
    } catch (error) {
      showError('还原失败');
    } finally {
      setLoading(false);
    }
  };

  const importBackup = async () => {
    if (!importFile) {
      showError('请先选择备份文件');
      return;
    }
    const formData = new FormData();
    formData.append('file', importFile);
    setLoading(true);
    try {
      const res = await API.post('/api/jdc/backup/import', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (res.data.success) {
        showSuccess('备份已导入');
        setImportFile(null);
        load();
      }
    } catch (error) {
      showError('导入失败');
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id' },
    { title: '文件名', dataIndex: 'filename' },
    { title: '类型', dataIndex: 'backup_type' },
    { title: '状态', dataIndex: 'status' },
    {
      title: '大小',
      render: (_, record) => <Text>{record.size_bytes || 0} bytes</Text>,
    },
    {
      title: '操作',
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 8 }}>
          <Button
            theme='light'
            type='primary'
            size='small'
            onClick={() => window.open(`/api/jdc/backup/download/${record.id}`, '_blank')}
          >
            下载
          </Button>
          <Button theme='light' size='small' onClick={() => restoreBackup(record.id)}>
            还原
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Spin spinning={loading}>
      <Card>
        <Banner
          type='info'
          description='每天定时把当前业务数据库导出为一个独立 SQLite 快照文件，可下载、导入到新机器，也可从已有快照一键还原。'
          closeIcon={null}
        />
        <Form
          getFormApi={(api) => {
            formRef.current = api;
          }}
          style={{ marginTop: 16 }}
        >
          <Form.Switch
            field='jdc_backup.backup_enabled'
            label='启用每日自动备份'
            checked={inputs['jdc_backup.backup_enabled']}
            onChange={updateField('jdc_backup.backup_enabled')}
          />
          <Form.Input
            field='jdc_backup.backup_time'
            label='备份时间'
            placeholder='00:00'
            value={inputs['jdc_backup.backup_time']}
            onChange={updateField('jdc_backup.backup_time')}
          />
          <Form.Input
            field='jdc_backup.backup_dir'
            label='备份目录'
            placeholder='留空则默认 logs/jdc-backups'
            value={inputs['jdc_backup.backup_dir']}
            onChange={updateField('jdc_backup.backup_dir')}
          />
          <Form.InputNumber
            field='jdc_backup.retain_count'
            label='保留备份数量'
            min={1}
            value={inputs['jdc_backup.retain_count']}
            onChange={updateField('jdc_backup.retain_count')}
          />
          <Form.Switch
            field='jdc_backup.cleanup_enabled'
            label='备份后顺带清理数据库日志'
            checked={inputs['jdc_backup.cleanup_enabled']}
            onChange={updateField('jdc_backup.cleanup_enabled')}
          />
          <Form.InputNumber
            field='jdc_backup.log_retention_days'
            label='日志保留天数'
            min={1}
            value={inputs['jdc_backup.log_retention_days']}
            onChange={updateField('jdc_backup.log_retention_days')}
          />
          <Form.InputNumber
            field='jdc_backup.quota_data_retention_days'
            label='quota_data 保留天数'
            min={1}
            value={inputs['jdc_backup.quota_data_retention_days']}
            onChange={updateField('jdc_backup.quota_data_retention_days')}
          />
          <Form.Switch
            field='jdc_backup.include_logs_in_export'
            label='导出时包含 logs 和 quota_data'
            checked={inputs['jdc_backup.include_logs_in_export']}
            onChange={updateField('jdc_backup.include_logs_in_export')}
          />
        </Form>
        <div style={{ display: 'flex', gap: 8, marginTop: 16, flexWrap: 'wrap' }}>
          <Button type='primary' onClick={save}>保存设置</Button>
          <Button onClick={createBackup}>立即备份</Button>
          <Button onClick={cleanupLogs}>清理数据库日志</Button>
          <Button onClick={load}>刷新</Button>
        </div>
      </Card>

      <Card style={{ marginTop: 12 }}>
        <Text strong>导入备份</Text>
        <div style={{ marginTop: 12, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <input type='file' accept='.sqlite,.db' onChange={(e) => setImportFile(e.target.files?.[0] || null)} />
          <Button type='primary' onClick={importBackup}>上传并导入</Button>
          {importFile ? <Text>{importFile.name}</Text> : null}
        </div>
      </Card>

      <Card style={{ marginTop: 12 }}>
        <Text strong>备份记录</Text>
        <Table style={{ marginTop: 12 }} columns={columns} dataSource={records} pagination={false} rowKey='id' />
      </Card>
    </Spin>
  );
}
