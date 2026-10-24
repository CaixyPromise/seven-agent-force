"use client";

import React from "react";
import {ProFormText} from "@ant-design/pro-components";
import {LockOutlined, UserOutlined} from "@ant-design/icons";
import {ACCOUNT_REGEX, PASSWORD_REGEX} from "@/constants/regex";

interface AuthFormProps {
  isAccountLocked?: boolean
  onAccountBlur?: (account: string) => void
}

const AuthForm: React.FC<AuthFormProps> = ({isAccountLocked = false, onAccountBlur}) => {
  return <>
    <ProFormText
      name="userAccount-Login"
      fieldProps={{
        size: 'large',
        prefix: <UserOutlined/>,
        disabled: isAccountLocked,
        autoComplete: 'username',
        autoCapitalize: 'none',
        autoCorrect: 'off',
        onBlur: (event: React.FocusEvent<HTMLInputElement>) => {
          onAccountBlur?.(event.target.value);
        },
      }}
      placeholder={'请输入账号'}
      rules={[
        {
          required: true,
          message: '账号是必填项！',
        },
        {
          min: 4,
          message: '账号长度至少4位！',
        },
        {
          max: 50,
          message: '账号长度不能超过50位！',
        },
        ACCOUNT_REGEX
      ]}
    />
    <ProFormText.Password
      name="userPassword-Login"
      fieldProps={{
        size: 'large',
        prefix: <LockOutlined/>,
        disabled: isAccountLocked,
        autoComplete: 'current-password',
      }}
      placeholder={'请输入密码'}
      rules={[
        {
          required: true,
          message: '密码是必填项！',
        },
        {
          min: 8,
          message: '密码长度至少8位！',
        },
        {
          max: 20,
          message: '密码长度不能超过20位！',
        },
        PASSWORD_REGEX
      ]}
    />
  </>
}

export default AuthForm;
