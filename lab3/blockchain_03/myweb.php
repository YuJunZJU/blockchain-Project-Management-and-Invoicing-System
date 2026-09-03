<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>this is my first PHP web</title>
</head>
<body>
    <h1>hello web</h1>
    <p>what to eat today? That is a big question</p>

    <hr>
    <h3>this is a smaller header</h3>
    <p>now it's 2050.2.31</p> <?php
    echo "<h3>this is produced by php!</h3>";
    
    // 获取当前服务器时间
    $curtime = date('Y-m-d H:i:s');

    // 使用 . 来拼接字符串，并修正标签方向
    echo "<p>Now it's..let me see..oh it's " . $curtime . "</p>"; 
    ?>
</body>
</html>
