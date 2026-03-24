import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Spin,
  Table,
  TagInput,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, toBoolean } from '../../helpers';

const { Text, Paragraph } = Typography;

const DEFAULT_INPUTS = {
  'jdc_tg.bot_enabled': false,
  'jdc_tg.bot_token': '',
  'jdc_tg.admin_ids': [],
  'jdc_tg.auto_adjust_enabled': false,
  'jdc_tg.auto_adjust_time': '00:00',
  'jdc_tg.low_quota_threshold': 20,
  'jdc_tg.low_quota_target': 100,
  'jdc_tg.high_quota_threshold': 200,
  'jdc_tg.high_quota_target': 80,
  'jdc_tg.adjust_whitelist': [],
  'jdc_tg.daily_report_enabled': true,
};

export default function JdcTgSetting() {
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(DEFAULT_INPUTS);
  const [logs, setLogs] = useState([]);
  const [lastReport, setLastReport] = useState('');
  const formRef = useRef(null);

  const parseArrayValue = (value) => {
    if (!value) return [];
    try {
      const parsed = JSON.parse(value);
      return Array.isArray(parsed) ? parsed : [];
    } catch (e) {
      return [];
    }
  };

  const load = async () => {
    setLoading(true);
    try {
      const [optionsRes, logsRes] = await Promise.all([
        API.get('/api/option/'),
        API.get('/api/jdc/tg/adjustment_logs'),
      ]);
      if (optionsRes.data.success) {
        const nextInputs = { ...DEFAULT_INPUTS };
        optionsRes.data.data.forEach((item) => {
          if (!(item.key in nextInputs)) return;
          if (typeof nextInputs[item.key] === 'boolean') {
            nextInputs[item.key] = toBoolean(item.value);
          } else if (Array.isArray(nextInputs[item.key])) {
            nextInputs[item.key] = parseArrayValue(item.value);
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
      if (logsRes.data.success) {
        setLogs(logsRes.data.data || []);
      }
    } catch (error) {
      showError('加载 JDC TG 设置失败');
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
    const updates = Object.entries(inputs)
      .filter(([key, value]) => {
        if (key === 'jdc_tg.bot_token' && !value) {
          return false;
        }
        return true;
      })
      .map(([key, value]) =>
        API.put('/api/option/', {
          key,
          value: Array.isArray(value)
            ? JSON.stringify(value)
            : typeof value === 'boolean'
              ? String(value)
              : String(value ?? ''),
        }),
      );
    setLoading(true);
    try {
      await Promise.all(updates);
      showSuccess('JDC TG 设置已保存');
      load();
    } catch (error) {
      showError('保存失败');
    } finally {
      setLoading(false);
    }
  };

  const sendTest = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/jdc/tg/send_test');
      if (res.data.success) {
        showSuccess('测试消息已发送');
      }
    } catch (error) {
      showError('发送测试消息失败');
    } finally {
      setLoading(false);
    }
  };

  const runAdjustment = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/jdc/tg/run_adjustment');
      if (res.data.success) {
        setLastReport(res.data.data?.report || '');
        showSuccess('额度处理完成');
        load();
      }
    } catch (error) {
      showError('执行失败');
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    { title: '日期', dataIndex: 'task_date' },
    { title: '用户', dataIndex: 'username' },
    { title: '动作', dataIndex: 'action' },
    { title: '原因', dataIndex: 'reason' },
    { title: '处理前', dataIndex: 'before_quota' },
    { title: '处理后', dataIndex: 'after_quota' },
    { title: '变动值', dataIndex: 'delta_quota' },
  ];

  return (
    <Spin spinning={loading}>
      <Card>
        <Banner
          type='info'
          description='配置 Telegram Bot Token、管理员 ID、每日 0 点额度处理规则和白名单。Bot 支持查询统计、查看用户、加减额度、生成兑换码。'
          closeIcon={null}
        />
        <Form
          getFormApi={(api) => {
            formRef.current = api;
          }}
          style={{ marginTop: 16 }}
        >
          <Form.Switch
            field='jdc_tg.bot_enabled'
            label='启用 TG Bot'
            checked={inputs['jdc_tg.bot_enabled']}
            onChange={updateField('jdc_tg.bot_enabled')}
          />
          <Form.Input
            field='jdc_tg.bot_token'
            label='TG Bot Token'
            placeholder='留空表示不修改已保存 token'
            value={inputs['jdc_tg.bot_token']}
            onChange={updateField('jdc_tg.bot_token')}
          />
          <Form.Slot label='管理员 ID'>
            <TagInput value={inputs['jdc_tg.admin_ids']} onChange={updateField('jdc_tg.admin_ids')} placeholder='输入管理员 Telegram 数字 ID 后回车，不是 @用户名' />
          </Form.Slot>
          <Paragraph type='tertiary'>
            先在 Telegram 里给机器人发送 /start，再填写你的数字 Telegram 用户 ID；否则测试消息和斜杠命令都不会生效。
          </Paragraph>
          <Form.Switch
            field='jdc_tg.auto_adjust_enabled'
            label='启用每日自动额度处理'
            checked={inputs['jdc_tg.auto_adjust_enabled']}
            onChange={updateField('jdc_tg.auto_adjust_enabled')}
          />
          <Form.Input
            field='jdc_tg.auto_adjust_time'
            label='执行时间'
            value={inputs['jdc_tg.auto_adjust_time']}
            onChange={updateField('jdc_tg.auto_adjust_time')}
          />
          <Form.InputNumber
            field='jdc_tg.low_quota_threshold'
            label='低余额阈值'
            value={inputs['jdc_tg.low_quota_threshold']}
            onChange={updateField('jdc_tg.low_quota_threshold')}
          />
          <Form.InputNumber
            field='jdc_tg.low_quota_target'
            label='低余额补到多少'
            value={inputs['jdc_tg.low_quota_target']}
            onChange={updateField('jdc_tg.low_quota_target')}
          />
          <Form.InputNumber
            field='jdc_tg.high_quota_threshold'
            label='高余额阈值'
            value={inputs['jdc_tg.high_quota_threshold']}
            onChange={updateField('jdc_tg.high_quota_threshold')}
          />
          <Form.InputNumber
            field='jdc_tg.high_quota_target'
            label='高余额压到多少'
            value={inputs['jdc_tg.high_quota_target']}
            onChange={updateField('jdc_tg.high_quota_target')}
          />
          <Form.Slot label='白名单账号'>
            <TagInput value={inputs['jdc_tg.adjust_whitelist']} onChange={updateField('jdc_tg.adjust_whitelist')} placeholder='输入用户名后回车' />
          </Form.Slot>
          <Form.Switch
            field='jdc_tg.daily_report_enabled'
            label='每日处理后推送管理员详细报告'
            checked={inputs['jdc_tg.daily_report_enabled']}
            onChange={updateField('jdc_tg.daily_report_enabled')}
          />
        </Form>
        <div style={{ display: 'flex', gap: 8, marginTop: 16, flexWrap: 'wrap' }}>
          <Button type='primary' onClick={save}>保存设置</Button>
          <Button onClick={sendTest}>发送测试消息</Button>
          <Button onClick={runAdjustment}>立即执行额度处理</Button>
          <Button onClick={load}>刷新</Button>
        </div>
      </Card>

      {lastReport ? (
        <Card style={{ marginTop: 12 }}>
          <Text strong>最近一次执行报告</Text>
          <Paragraph style={{ whiteSpace: 'pre-wrap', marginTop: 12 }}>{lastReport}</Paragraph>
        </Card>
      ) : null}

      <Card style={{ marginTop: 12 }}>
        <Text strong>最近处理日志</Text>
        <Table style={{ marginTop: 12 }} columns={columns} dataSource={logs} pagination={false} rowKey='id' />
      </Card>
    </Spin>
  );
}
